package queue

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/panjf2000/ants/v2"
	"github.com/supersupersimple/queue/ent"
	"github.com/supersupersimple/queue/ent/job"
)

type EnqueueJob struct {
	QueueName     string
	RefID         string
	Priority      uint
	JobBody       []byte
	ScheduledTime *time.Time
}

type Queue interface {
	Start()
	Stop()

	Enqueue(ctx context.Context, j *EnqueueJob) error
}

type Handler func(ctx context.Context, queueName string, refId string, jobBody []byte) error

type Logger interface {
	Printf(format string, args ...interface{})
}

type queue struct {
	db      *sql.DB
	config  *QueueConfig
	handler Handler

	client *ent.Client

	done     chan struct{}
	pool     *ants.Pool
	ackQueue chan *ent.Job

	disabledQueues []string

	logger Logger
}

var (
	logLmsgprefix = 64
	defaultLogger = Logger(log.New(os.Stderr, "[queue]: ", log.LstdFlags|logLmsgprefix|log.Lmicroseconds))
)

func New(db *sql.DB, config *QueueConfig, h Handler) (Queue, error) {
	q := &queue{
		db:       db,
		config:   config,
		handler:  h,
		done:     make(chan struct{}),
		ackQueue: make(chan *ent.Job, 3*config.WorkerPoolSize),
	}

	if err := q.initDB(); err != nil {
		return nil, err
	}

	q.loadConfig(config)

	if config.Logger != nil {
		q.logger = config.Logger
	} else {
		q.logger = defaultLogger
	}

	return q, nil
}

func (q *queue) initDB() error {
	drv := entsql.OpenDB("sqlite3", q.db)
	q.client = ent.NewClient(ent.Driver(drv))
	// Run the auto migration tool.
	err := q.client.Schema.Create(context.Background())
	if err != nil {
		return err
	}

	q.pool, err = ants.NewPool(q.config.WorkerPoolSize,
		ants.WithLogger(q.logger),
		ants.WithNonblocking(false),
		ants.WithMaxBlockingTasks(3*q.config.WorkerPoolSize),
	)
	if err != nil {
		return err
	}

	return nil
}

func (q *queue) Stop() {
	q.pool.Release()
	q.done <- struct{}{}
	q.client.Close()
}

func (q *queue) Start() {
	pickJobTicker := time.NewTicker(q.config.PickJobsInterval)
	deleteJobTicker := time.NewTicker(q.config.DeleteJobSetting.JobInterval)
	if !q.config.DeleteJobSetting.DeleteSuccessfulJob {
		deleteJobTicker.Stop()
	}
	rePickJobTicket := time.NewTicker(q.config.RePickProcessingJobInterval)

	for {
		select {
		case <-q.done:
			return
		case <-pickJobTicker.C:
			q.pickAndSubmit(false)
		case j := <-q.ackQueue:
			q.markJob(j)
		case <-rePickJobTicket.C:
			q.pickAndSubmit(true)
		case <-deleteJobTicker.C:
			q.deleteJobs()
		}
	}
}

func (q *queue) submitJobs(jobs []*ent.Job) {
	for _, j := range jobs {
		err := q.pool.Submit(func() {
			var err error
			defer func() {
				if r := recover(); r != nil {
					q.logger.Printf("recovered from panic. err: %s", r)
					err = fmt.Errorf("panic %s", r)
				}
				q.postHandle(j, err)
			}()
			ctx := context.Background()
			err = q.handler(ctx, j.QueueName, j.RefID, []byte(j.Body))
		})
		if err != nil {
			q.logger.Printf("submit job err: %s", err.Error())
		}
	}
}

func (q *queue) postHandle(j *ent.Job, err error) {
	if err == nil {
		j.Status = job.StatusSuccessful
	} else {
		q.logger.Printf("job execute failed. id=%d err=%s", j.ID, err.Error())
		j.Error = err.Error()
		// check retry times
		config, ok := q.config.QueueSetting[j.QueueName]
		if !ok {
			config = q.config.DefaultQueueSetting
		}
		if j.RetryTimes < config.RetryTimes {
			j.Status = job.StatusWaitRetry
			j.RetryTimes = j.RetryTimes + 1
			j.ScheduledAt = time.Now().Add(config.RetryInterval)
		} else {
			j.Status = job.StatusFailed
		}
	}
	q.ackQueue <- j
}

func (q *queue) Enqueue(ctx context.Context, j *EnqueueJob) error {
	jc := q.client.Job.Create().SetBody(string(j.JobBody))
	if j.ScheduledTime != nil {
		jc.SetScheduledAt(*j.ScheduledTime)
	}
	if j.QueueName != "" {
		jc.SetQueueName(j.QueueName)
	}
	if j.RefID != "" {
		jc.SetRefID(j.RefID)
	}
	if j.Priority > 0 {
		jc.SetPriority(j.Priority)
	}
	err := jc.Exec(ctx)
	if err != nil {
		q.logger.Printf("insert job error. err=%s", err.Error())
		return err
	}
	q.logger.Printf("inserted 1 job")
	return nil
}

func (q *queue) pickAndSubmit(rePick bool) {
	limit := q.config.WorkerPoolSize
	if q.pool.Waiting() > 0 {
		limit = q.config.WorkerPoolSize - q.pool.Waiting()
	}
	var statuses []job.Status
	if rePick {
		statuses = []job.Status{job.StatusProcessing}
	} else {
		statuses = []job.Status{job.StatusInit, job.StatusWaitRetry}
	}
	jobs := q.pickJobs(rePick, statuses, limit)
	if len(jobs) > 0 {
		go q.submitJobs(jobs)
	}
}

func (q *queue) pickJobs(rePick bool, statuses []job.Status, limit int) []*ent.Job {
	if limit <= 0 {
		return nil
	}
	ctx := context.Background()
	var returnJobs []*ent.Job
	scheduledAt := time.Now()
	if rePick {
		scheduledAt = scheduledAt.Add(-q.config.RePickProcessingJobDelay) // to pick up that over delay time still on processing jobs
	}
	if err := withTx(ctx, q.client, func(tx *ent.Tx) error {
		jobs, err := q.queryJobs(tx, scheduledAt, statuses, limit)
		if err != nil {
			return err
		}

		if len(jobs) == 0 {
			return nil
		}

		ids := make([]uint64, 0, len(jobs))
		for _, j := range jobs {
			ids = append(ids, j.ID)
			returnJobs = append(returnJobs, j)
		}
		err = q.updateJobStatus(tx, ids)
		return err
	}); err != nil {
		q.logger.Printf("picked jobs err %v", err)
		return nil
	}
	if len(returnJobs) > 0 {
		q.logger.Printf("picked %d jobs", len(returnJobs))
	}
	return returnJobs
}

func (q *queue) queryJobs(tx *ent.Tx, scheduledAt time.Time, statuses []job.Status, limit int) ([]*ent.Job, error) {
	return tx.Job.Query().Where(
		job.ScheduledAtLTE(scheduledAt),
		job.QueueNameNotIn(q.disabledQueues...),
		job.StatusIn(statuses...),
	).
		Order(
			job.ByPriority(entsql.OrderDesc()),
			job.ByScheduledAt(),
		).
		Limit(limit).All(context.Background())
}

func (q *queue) updateJobStatus(tx *ent.Tx, ids []uint64) error {
	return tx.Job.Update().Where(
		job.IDIn(ids...),
	).
		SetStatus(job.StatusProcessing).
		Exec(context.Background())
}

func (q *queue) markJob(j *ent.Job) {
	ctx := context.Background()

	var err error
	if j.Status == job.StatusWaitRetry {
		err = q.client.Job.UpdateOne(j).
			SetStatus(j.Status).
			SetError(j.Error).
			SetScheduledAt(j.ScheduledAt).
			SetRetryTimes(j.RetryTimes).
			Exec(ctx)
	} else {
		err = q.client.Job.UpdateOne(j).
			SetStatus(j.Status).
			SetFinishedAt(time.Now()).
			Exec(ctx)
	}
	if err != nil {
		q.logger.Printf("update job failed. id=%d err=%s", j.ID, err.Error())
		return
	}
	q.logger.Printf("updated job id=%d status=%s", j.ID, j.Status.String())
}

func (q *queue) loadConfig(config *QueueConfig) {
	for queue, s := range config.QueueSetting {
		if s.Disable {
			q.disabledQueues = append(q.disabledQueues, queue)
		}
	}
}

func (q *queue) deleteJobs() {
	ctx := context.Background()
	c, _ := q.client.Job.Delete().Where(
		job.FinishedAtLT(time.Now().Add(-q.config.DeleteJobSetting.StoreTime)),
		job.StatusIn(job.StatusSuccessful),
	).Exec(ctx)

	if c > 0 {
		q.logger.Printf("deleted %d jobs\n", c)
	}
}
