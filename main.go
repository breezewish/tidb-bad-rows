package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/go-sql-driver/mysql"
	"go.uber.org/atomic"
)

var fHost = flag.String("host", "127.0.0.1", "Host")
var fPort = flag.Int("port", 4000, "Port")
var fUser = flag.String("user", "root", "Username")
var fPass = flag.String("pass", "", "Password")
var fDatabase = flag.String("db", "test", "Database name")
var fTable = flag.String("table", "", "Table name")
var fConcurrency = flag.Int("concurrency", 2, "Scan concurrency")
var fFilter = flag.String("projection", "*", "Projection clause to be used in scanning")

var db *sql.DB

var pool *workerpool.WorkerPool
var pendingTasks atomic.Uint32
var finishedTasks atomic.Uint32
var brokenRows atomic.Uint32

func submitNewTask(minRowID uint64, maxRowID uint64) {
	if minRowID >= maxRowID {
		return
	}
	pendingTasks.Inc()
	fmt.Printf("+ Range[%d, %d) \t - New\n", minRowID, maxRowID)
	pool.Submit(func() {
		fmt.Printf("+ Range[%d, %d) \t - Scanning\n", minRowID, maxRowID)
		tBegin := time.Now()
		err := scanRange(minRowID, maxRowID)
		tElapsed := time.Since(tBegin)
		if err == nil {
			fmt.Printf("+ Range[%d, %d) \t - Data is OK (elapsed %fs)\n", minRowID, maxRowID, tElapsed.Seconds())
		} else {
			fmt.Printf("+ Range[%d, %d) \t - Data is broken (elapsed %fs)\n", minRowID, maxRowID, tElapsed.Seconds())
			if minRowID+1 == maxRowID {
				// This is the only broken row
				brokenRows.Inc()
				fmt.Printf("+ Discovered broken row, _tidb_rowid = %d\n", minRowID)
			} else {
				midRowID := minRowID + (maxRowID-minRowID)/2
				submitNewTask(minRowID, midRowID)
				submitNewTask(midRowID, maxRowID)
			}
		}
		finishedTasks.Inc()
		pendingTasks.Dec()
	})
}

func scanRange(minRowID uint64, maxRowID uint64) error {
	rows, err := db.Query(fmt.Sprintf("EXPLAIN ANALYZE SELECT %s FROM %s WHERE _tidb_rowid >= %d AND _tidb_rowid < %d", *fFilter, *fTable, minRowID, maxRowID))
	if err != nil {
		return err
	}
	row := rows.Next()
	if !row {
		_ = rows.Close()
		return fmt.Errorf("cannot read next row")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_ = rows.Close()
	return nil
}

func main() {
	flag.Parse()

	dsnConfig := mysql.NewConfig()
	dsnConfig.User = *fUser
	dsnConfig.Passwd = *fPass
	dsnConfig.Net = "tcp"
	dsnConfig.Addr = fmt.Sprintf("%s:%d", *fHost, *fPort)
	dsnConfig.DBName = *fDatabase

	if len(*fTable) == 0 {
		fmt.Println("Please specify the table name using -table=<TABLE_NAME>")
		os.Exit(1)
		return
	}

	dsn := dsnConfig.FormatDSN()
	fmt.Printf("+ Connecting to %s\n", dsn)
	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		panic(err)
	}

	fmt.Printf("+ Reading MIN(_tidb_rowid)\n")
	var minRowID uint64
	err = db.QueryRow(fmt.Sprintf("SELECT MIN(_tidb_rowid) FROM %s", *fTable)).Scan(&minRowID)
	if err != nil {
		panic(err)
	}

	fmt.Printf("  - MIN(%s._tidb_rowid) = %d\n", *fTable, minRowID)

	fmt.Printf("+ Reading MAX(_tidb_rowid)\n")
	var maxRowID uint64
	err = db.QueryRow(fmt.Sprintf("SELECT MAX(_tidb_rowid) FROM %s", *fTable)).Scan(&maxRowID)
	if err != nil {
		panic(err)
	}

	fmt.Printf("  - MAX(%s._tidb_rowid) = %d\n", *fTable, maxRowID)

	if maxRowID < minRowID {
		panic("Unexpected error: maxRowID is not >= minRowID")
	}

	pool = workerpool.New(*fConcurrency)
	submitNewTask(minRowID, maxRowID+1)

	// Task tracker
	for {
		select {
		case <-time.After(time.Second):
			fmt.Printf("+ Task statistics: %d pending tasks, %d finished tasks, %d broken rows\n",
				pendingTasks.Load(),
				finishedTasks.Load(),
				brokenRows.Load())
			if pendingTasks.Load() == 0 {
				fmt.Println("+ All tasks are finished")
				os.Exit(0)
			}
		}
	}
}
