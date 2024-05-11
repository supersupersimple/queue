# Super Super Simple Queue

This readme doc was wrote by AI.

This is a simple super implementation of a queue management system using SQLite. It can be used to handle job queuing, processing, and retry mechanisms.

## Table of Contents

- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [Running the Application](#running-the-application)
- [Usage](#usage)
  * [Enqueuing Jobs](#enqueuing-jobs)
  * [Handling Jobs](#handling-jobs)
- [Contributing](#contributing)
- [License](#license)
- [Acknowledgments](#acknowledgments)

## Getting Started

To get started with the Queue Management System, follow these steps:

1. **Install dependencies:**
   - Install Go 1.22 or later
   - Sqlite library

2. **Install the packages:**

   Run the following command to install the required packages in the go.mod file.

   ```
   go get github.com/supersupersimple/queue
   ```

## Configuration

   QueueConfig
   ``` go
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
   ```

## Running the Example Application

To run the application, perform the following steps:

1. Navigate to the project's directory.
2. Install the dependencies, if you haven't done so already.
   ```
   go mod download
   ```
3. Run the binary:

   ```
   go run example/main.go
   ```

## Usage

### Init and Start Queue

Call `New` and `Start` functions to initialize and start the queue.

### Enqueuing Jobs

Enqueuing jobs can be done by calling the `Enqueue` function. The function takes a `Job` struct as an argument. And pass `Job` when job was executed.

MENTION: the queue name + ref id should be unique for all jobs. 

#### EnqueueJob structure

- `JobBody`: (*[]byte*) Job data or payload in bytes. You can define your job body format.

#### Example

```go
enqueueJob := &queue.EnqueueJob{
    QueueName:     "SampleQueue", // if not set, will be default
    RefID:        "SampleRefID", // if not set, will generate automatically
    Priority:      0, // if not set, will be 0. Higher priority will be executed first
    JobBody:       []byte("Sample job body"), // must set
    ScheduledTime: time.Now().Add(10 * time.Minute), // if not set, will be scheduled immediately
}
err := queue.Enqueue(context.Background(), enqueueJob)
if err != nil {
    log.Printf("Failed to enqueue job: %v", err)
}
```

### Handling Jobs

When job was triggered, the handle func will be executed. The handle func takes a `ctx`, `queueName`, `refID`, and `jobBody` as arguments.

```go
func Handler(ctx context.Context, queueName, refID string, jobBody []byte) error {
	// process the job
	return nil // if return err or panic, will mark this job as failed or retry this job that depends on the queue config
}
```

## Contributing

Contributions are welcome! Please submit pull requests or open issues for any improvements or additions you would like to see in the project. Ensure your changes are tested thoroughly and follow the existing coding style before submitting a pull request.

## License

This project is licensed under the MIT License - see the [LICENSE.md](LICENSE.md) file for details.

## Acknowledgments

This project was built leveraging the following libraries:

- [entgo.io/ent](https://github.com/entgo/ent)
- [github.com/panjf2000/ants/v2](https://github.com/panjf2000/ants)

Feel free to suggest any updates or edits by submitting a pull request. Your contributions are welcome!