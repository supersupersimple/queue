package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/supersupersimple/queue"
)

var (
	wg *sync.WaitGroup
)

func main() {
	db, err := sql.Open("sqlite3", "queue.db?_fk=1&_journal_mode=wal")
	if err != nil {
		log.Fatalf("failed opening connection to sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	c := queue.NewDefaultConfig()
	q, err := queue.New(db, c, workfunc)
	if err != nil {
		log.Fatalf("init queue failed %v", err)
	}

	wg = &sync.WaitGroup{}

	go q.Start()

	addJobs(q)
	wg.Wait()

	time.Sleep(time.Second)

	q.Stop()
}

func addJobs(q queue.Queue) {
	for i := 0; i < 100; i++ {
		wg.Add(1)
		id := strconv.FormatInt(int64(i), 10)
		err := q.Enqueue(context.TODO(), &queue.EnqueueJob{
			JobBody: []byte(id),
		})
		if err != nil {
			fmt.Println(err)
		}
		time.Sleep(1 * time.Millisecond)
	}
}

func workfunc(ctx context.Context, _, _ string, jobBody []byte) error {
	time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
	fmt.Printf("finish %s\n", jobBody)
	wg.Done()
	return nil
}
