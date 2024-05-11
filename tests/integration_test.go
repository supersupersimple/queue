package tests

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/suite"
	"github.com/supersupersimple/queue"
	"github.com/supersupersimple/queue/ent"
	"github.com/supersupersimple/queue/ent/job"
	_ "modernc.org/sqlite"
)

func TestSuite(t *testing.T) {
	suite.Run(t, &queueTestSuite{})
}

type queueTestSuite struct {
	suite.Suite
}

func workfunc(ctx context.Context, queueName, refId string, jobBody []byte) error {
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (s *queueTestSuite) SetupSuite() {
	os.Mkdir("./db", os.ModePerm)
}

func (s *queueTestSuite) TearDownSuite() {
	os.RemoveAll("./db/")
}

func (s *queueTestSuite) initDB(dbName string) *sql.DB {
	db, err := sql.Open("sqlite", "./db/"+dbName+"?_pragma=foreign_keys(1)&_pragma=journal_mode(wal)")
	if err != nil {
		log.Fatalf("failed opening connection to sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func (s *queueTestSuite) initQueue(db *sql.DB, config *queue.QueueConfig, fn queue.Handler) queue.Queue {
	if config == nil {
		config = queue.NewDefaultConfig()
	}
	if fn == nil {
		fn = workfunc
	}
	q, err := queue.New(db, config, fn)
	s.NoError(err)
	return q
}

func (s *queueTestSuite) getEntClient(db *sql.DB) *ent.Client {
	drv := entsql.OpenDB("sqlite3", db)
	client := ent.NewClient(ent.Driver(drv))
	return client
}

func (s *queueTestSuite) TestHappyFlow() {
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	handler := func(ctx context.Context, queueName, refId string, jobBody []byte) error {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
		return nil
	}
	db := s.initDB("happy_flow.db")
	defer db.Close()
	q := s.initQueue(db, nil, handler)
	go q.Start()
	defer q.Stop()

	wg.Add(1)
	err := q.Enqueue(ctx, &queue.EnqueueJob{})
	s.NoError(err)

	wg.Wait()

	// verify job status
	client := s.getEntClient(db)
	jobs := client.Job.Query().AllX(ctx)
	s.Len(jobs, 1)
	for jobs[0].Status != job.StatusSuccessful {
		time.Sleep(10 * time.Millisecond)
		jobs = client.Job.Query().AllX(ctx)
	}
	s.Equal(job.StatusSuccessful, jobs[0].Status)
}

func (s *queueTestSuite) TestReturnErrorAndRetry() {
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	handler := func(ctx context.Context, queueName, refId string, jobBody []byte) error {
		wg.Done()
		return errors.New("mock error")
	}
	db := s.initDB("error_retry.db")
	defer db.Close()
	config := queue.NewDefaultConfig()
	config.DefaultQueueSetting.RetryInterval = time.Second
	q := s.initQueue(db, config, handler)
	go q.Start()
	defer q.Stop()

	wg.Add(3)
	err := q.Enqueue(ctx, &queue.EnqueueJob{})
	s.NoError(err)

	wg.Wait()

	// verify job status
	client := s.getEntClient(db)
	jobs := client.Job.Query().AllX(ctx)
	s.Len(jobs, 1)
	for jobs[0].Status != job.StatusFailed {
		time.Sleep(10 * time.Millisecond)
		jobs = client.Job.Query().AllX(ctx)
	}
	s.Equal(uint(2), jobs[0].RetryTimes)
	s.Contains(jobs[0].Error, "mock error")
}

func (s *queueTestSuite) TestPanicAndRetry() {
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	handler := func(ctx context.Context, queueName, refId string, jobBody []byte) error {
		wg.Done()
		panic("mock panic")
	}
	db := s.initDB("panic_retry.db")
	defer db.Close()
	config := queue.NewDefaultConfig()
	config.DefaultQueueSetting.RetryInterval = time.Second
	config.DefaultQueueSetting.RetryTimes = 3
	q := s.initQueue(db, config, handler)
	go q.Start()
	defer q.Stop()

	wg.Add(4) // 1+3
	err := q.Enqueue(ctx, &queue.EnqueueJob{})
	s.NoError(err)

	wg.Wait()

	// verify job status
	client := s.getEntClient(db)
	jobs := client.Job.Query().AllX(ctx)
	s.Len(jobs, 1)
	for jobs[0].Status != job.StatusFailed {
		time.Sleep(10 * time.Millisecond)
		jobs = client.Job.Query().AllX(ctx)
	}
	s.Equal(uint(3), jobs[0].RetryTimes)
	s.Contains(jobs[0].Error, "mock panic")
}

func (s *queueTestSuite) TestDeleteJobs() {
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	handler := func(ctx context.Context, queueName, refId string, jobBody []byte) error {
		wg.Done()
		return nil
	}
	db := s.initDB("delete_jobs.db")
	defer db.Close()
	config := queue.NewDefaultConfig()
	config.DeleteJobSetting.DeleteSuccessfulJob = true
	config.DeleteJobSetting.JobInterval = time.Second
	config.DeleteJobSetting.StoreTime = time.Second
	q := s.initQueue(db, config, handler)
	go q.Start()
	defer q.Stop()

	wg.Add(1)
	err := q.Enqueue(ctx, &queue.EnqueueJob{})
	s.NoError(err)

	wg.Wait()

	client := s.getEntClient(db)
	jobs := client.Job.Query().AllX(ctx)
	for len(jobs) > 0 {
		time.Sleep(10 * time.Millisecond)
		jobs = client.Job.Query().AllX(ctx)
	}
	s.Len(jobs, 0)
}

func (s *queueTestSuite) TestRePickJobs() {
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	handler := func(ctx context.Context, queueName, refId string, jobBody []byte) error {
		wg.Done()
		return nil
	}
	db := s.initDB("repick_jobs.db")
	defer db.Close()
	config := queue.NewDefaultConfig()
	config.RePickProcessingJobInterval = time.Second
	q := s.initQueue(db, config, handler)
	go q.Start()
	defer q.Stop()

	wg.Add(1)
	client := s.getEntClient(db)
	client.Job.Create().SetBody(" ").SetScheduledAt(time.Now().Add(-config.RePickProcessingJobDelay)).
		SetStatus(job.StatusProcessing).
		ExecX(ctx)
	wg.Wait()

	jobs := client.Job.Query().AllX(ctx)
	s.Len(jobs, 1)
	for jobs[0].Status != job.StatusSuccessful {
		time.Sleep(10 * time.Millisecond)
		jobs = client.Job.Query().AllX(ctx)
	}
	s.Equal(job.StatusSuccessful, jobs[0].Status)
}
