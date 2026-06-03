package store

import "github.com/ClickHouse/clickhouse-go/v2/lib/driver"

func New(conn driver.Conn) *Queries {
	return &Queries{conn: conn}
}

type Queries struct {
	conn driver.Conn
}
