package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/rs/xid"
)

// Job holds the schema definition for the Job entity.
type Job struct {
	ent.Schema
}

func (Job) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sssq_scheduled_jobs"},
	}
}

const (
	DefaultQueueName = "default"
)

// Fields of the Job.
func (Job) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),

		// field.Enum("type").Values("OneTime", "Recurring").Default("OneTime"),
		field.Enum("status").Values("Init", "Processing", "Successful", "Failed", "WaitRetry").Default("Init"),
		field.String("queue_name").Default(DefaultQueueName),
		field.String("ref_id").DefaultFunc(func() string {
			return xid.New().String()
		}),
		field.Uint("priority").Default(0),
		field.Uint("retry_times").Default(0),
		field.Text("body").Optional(),
		field.Text("error").Optional(),
		field.Time("scheduled_at").Default(time.Now),
		field.Time("finished_at").Optional(),
	}
}

// Edges of the Job.
func (Job) Edges() []ent.Edge {
	return nil
}

func (Job) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scheduled_at", "status", "queue_name", "priority"),
		index.Fields("ref_id", "queue_name").Unique(),
		index.Fields("finished_at", "queue_name"),
	}
}
