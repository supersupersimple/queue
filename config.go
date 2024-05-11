package queue

import "time"

func NewDefaultConfig() *QueueConfig {
	return &QueueConfig{
		DefaultQueueSetting: QueueSetting{
			Disable:       false,
			RetryTimes:    2,
			RetryInterval: 1 * time.Minute,
		},
		QueueSetting: map[string]QueueSetting{},

		WorkerPoolSize: 100,

		PickJobsInterval: time.Second,

		RePickProcessingJobInterval: 1 * time.Minute,
		RePickProcessingJobDelay:    20 * time.Minute,

		DeleteJobSetting: DeleteJobSetting{
			DeleteSuccessfulJob: true,
			JobInterval:         10 * time.Minute,
			StoreTime:           7 * 24 * time.Hour,
		},
	}
}

type QueueConfig struct {
	Logger Logger

	DefaultQueueSetting QueueSetting
	QueueSetting        map[string]QueueSetting

	WorkerPoolSize   int
	PickJobsInterval time.Duration

	RePickProcessingJobInterval time.Duration
	RePickProcessingJobDelay    time.Duration

	DeleteJobSetting DeleteJobSetting
}

type QueueSetting struct {
	Disable       bool
	RetryTimes    uint
	RetryInterval time.Duration
}

type DeleteJobSetting struct {
	DeleteSuccessfulJob bool
	JobInterval         time.Duration
	StoreTime           time.Duration
}
