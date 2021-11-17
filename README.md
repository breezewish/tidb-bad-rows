# tidb-bad-rows

A tool for finding corrupted data rows in TiDB. It scans the target table and using a divide and conquer paradigm to locate all corrupted data rows in the table that cannot decode successfully.

The following SQL statements will be executed:

```sql
EXPLAIN ANALYZE
  SELECT * FROM <TABLE_NAME> WHERE
  _tidb_rowid >= 1 AND _tidb_rowid < 100000

-- If [1, 100000) is corrupted:

EXPLAIN ANALYZE
  SELECT * FROM <TABLE_NAME> WHERE
  _tidb_rowid >= 1 AND _tidb_rowid < 50000

EXPLAIN ANALYZE
  SELECT * FROM <TABLE_NAME> WHERE
  _tidb_rowid >= 50000 AND _tidb_rowid < 100000

-- If [1, 50000) is corrupted:

EXPLAIN ANALYZE
  SELECT * FROM <TABLE_NAME> WHERE
  _tidb_rowid >= 1 AND _tidb_rowid < 25000

EXPLAIN ANALYZE
  SELECT * FROM <TABLE_NAME> WHERE
  _tidb_rowid >= 25000 AND _tidb_rowid < 50000

-- ......
```

## Build

```shell
go build -o bad_rows main.go
```

To cross compile for Linux x86_64:

```shell
env GOOS=linux GOARCH=amd64 go build -o bad_rows main.go
```

## Usage

```shell
./bad_rows -table <TABLE_NAME>
```

Usually you need to specify other parameters to establish a connection to the remote database:

```plain
  -concurrency int
    	Scan concurrency (default 2)
  -db string
    	Database name (default "test")
  -host string
    	Host (default "127.0.0.1")
  -pass string
    	Password
  -port int
    	Port (default 4000)
  -projection string
    	Projection clause to be used in scanning (default "*")
  -table string
    	Table name
  -user string
    	Username (default "root")
```

### Reduce Scanning Columns

In case of scanning all columns of all rows takes long time, you can reduce the cost by changing the column projection via `-projection`:

```shell
./bad_rows -table <TABLE_NAME> -projection "foo, bar, boz"
```

Which will invoke SQLs like:

```sql
EXPLAIN ANALYZE
  SELECT foo, bar, boz FROM .. WHERE
  _tidb_rowid >= .. AND _tidb_rowid < ..
```

## Interpret Outputs

This app will output the following rows for corrupted rows:

```plain
...
Discovered broken row, _tidb_rowid = .....
...
```

You can then checkout the content of this row by using:

```sql
SELECT <NOT_CORRUPTED_COLUMNS> FROM <TABLE_NAME> WHERE _tidb_rowid = <TIDB_ROWID>
```

You can also checkout the raw MVCC value (which is corrupted) of this row by using:

```sql
curl http://<TIDB_IP>:10080/mvcc/key/<DB_NAME>/<TABLE_NAME>/<TIDB_ROWID>
```

## Roadmap

- [ ] Support tables with user specified int primary keys
- [ ] Support cluster index
- [ ] Support specifying the row id upper bound and lower bound
