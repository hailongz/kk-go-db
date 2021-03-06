package kk

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
)

const DBFieldTypeString = 1
const DBFieldTypeInt = 2
const DBFieldTypeInt64 = 3
const DBFieldTypeDouble = 4
const DBFieldTypeBoolean = 5
const DBFieldTypeText = 6
const DBFieldTypeLongText = 7

type DBField struct {
	Length int
	Type   int
}

func (fd *DBField) DBType() string {
	switch fd.Type {
	case DBFieldTypeInt:
		if fd.Length == 0 {
			return "INT"
		}
		return fmt.Sprintf("INT(%d)", fd.Length)
	case DBFieldTypeInt64:
		if fd.Length == 0 {
			return "BIGINT"
		}
		return fmt.Sprintf("BIGINT(%d)", fd.Length)
	case DBFieldTypeDouble:
		if fd.Length == 0 {
			return "DOUBLE"
		}
		return fmt.Sprintf("DOUBLE(%d)", fd.Length)
	case DBFieldTypeBoolean:
		return "INT(1)"
	case DBFieldTypeText:
		if fd.Length == 0 {
			return "TEXT"
		}
		return fmt.Sprintf("TEXT(%d)", fd.Length)
	case DBFieldTypeLongText:
		if fd.Length == 0 {
			return "LONGTEXT"
		}
		return fmt.Sprintf("LONGTEXT(%d)", fd.Length)
	}
	if fd.Length == 0 {
		return "VARCHAR(45)"
	}
	return fmt.Sprintf("VARCHAR(%d)", fd.Length)
}

const DBIndexTypeAsc = 1
const DBIndexTypeDesc = 2

type DBIndex struct {
	Field  string
	Type   int
	Unique bool
}

func (idx *DBIndex) DBType() string {
	switch idx.Type {
	case DBIndexTypeAsc:
		return "ASC"
	case DBIndexTypeDesc:
		return "DESC"
	}
	return "ASC"
}

type DBTable struct {
	Name   string
	Key    string
	Fields map[string]DBField
	Indexs map[string]DBIndex
}

func DBInit(db *sql.DB) error {
	var _, err = db.Exec("CREATE TABLE IF NOT EXISTS __scheme (id BIGINT NOT NULL AUTO_INCREMENT,name VARCHAR(64) NULL,scheme TEXT NULL,PRIMARY KEY (id),INDEX name (name ASC) ) AUTO_INCREMENT=1;")
	return err
}

func DBBuild(db *sql.DB, table *DBTable, prefix string, auto_increment int) error {

	var tbname = prefix + table.Name

	var rs, err = db.Query("SELECT * FROM __scheme WHERE name=?", tbname)

	if err != nil {
		return err
	}

	defer rs.Close()

	if rs.Next() {

		var id int64
		var name string
		var scheme string
		rs.Scan(&id, &name, &scheme)
		var tb DBTable
		json.Unmarshal([]byte(scheme), &tb)
		var hasUpdate = false

		for name, field := range table.Fields {
			var fd, ok = tb.Fields[name]
			if ok {
				if fd.Type != field.Type || fd.Length != field.Length {
					_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s CHANGE %s %s %s;", tbname, name, name, field.DBType()))
					if err != nil {
						log.Fatal(err)
					}
					hasUpdate = true
				}
			} else {
				_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", tbname, name, field.DBType()))
				if err != nil {
					log.Fatal(err)
				}
				hasUpdate = true
			}
		}

		for name, index := range table.Indexs {
			var _, ok = tb.Indexs[name]
			if !ok {
				if index.Unique {
					_, err = db.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS  %s ON %s (%s %s);", name, tbname, name, index.DBType()))
					if err != nil {
						log.Fatal(err)
					}
				} else {
					_, err = db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS  %s ON %s (%s %s);", name, tbname, name, index.DBType()))
					if err != nil {
						log.Fatal(err)
					}
				}

				hasUpdate = true
			}
		}

		if hasUpdate {
			var b, _ = json.Marshal(table)
			_, err = db.Exec("UPDATE __scheme SET scheme=? WHERE id=?", string(b), id)
			if err != nil {
				log.Fatal(err)
			}
		}
	} else {

		var s bytes.Buffer
		var i int = 0

		s.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (", tbname))

		if table.Key != "" {
			s.WriteString(fmt.Sprintf("%s BIGINT NOT NULL AUTO_INCREMENT", table.Key))
			i += 1
		}

		for name, field := range table.Fields {
			if i != 0 {
				s.WriteString(",")
			}
			s.WriteString(fmt.Sprintf("%s %s", name, field.DBType()))
			i += 1
		}

		if table.Key != "" {
			s.WriteString(fmt.Sprintf(", PRIMARY KEY(%s) ", table.Key))
		}

		for name, index := range table.Indexs {

			if index.Unique {
				s.WriteString(fmt.Sprintf(",UNIQUE INDEX %s (%s %s)", name, name, index.DBType()))
			} else {
				s.WriteString(fmt.Sprintf(",INDEX %s (%s %s)", name, name, index.DBType()))
			}

		}

		if table.Key != "" {
			s.WriteString(fmt.Sprintf(" ) AUTO_INCREMENT = %d;", auto_increment))
		} else {
			s.WriteString(" ) ;")
		}

		log.Println(s.String())

		_, err = db.Exec(s.String())
		if err != nil {
			log.Fatal(err)
		}

		var b, _ = json.Marshal(table)

		_, err = db.Exec("INSERT INTO __scheme(name,scheme) VALUES(?,?)", tbname, string(b))
		if err != nil {
			log.Fatal(err)
		}
	}

	return nil
}

type DBQueryer interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

func DBQuery(db DBQueryer, table *DBTable, prefix string, sql string, args ...interface{}) (*sql.Rows, error) {
	var tbname = prefix + table.Name
	return db.Query(fmt.Sprintf("SELECT * FROM %s %s", tbname, sql), args...)
}

func DBUpdate(db *sql.DB, table *DBTable, prefix string, object interface{}) (sql.Result, error) {

	var tbname = prefix + table.Name
	var s bytes.Buffer
	var fsc = len(table.Fields)
	var fs = make([]interface{}, fsc+1)
	var key interface{} = nil
	var n = 0

	s.WriteString(fmt.Sprintf("UPDATE %s SET ", tbname))

	var tbv = reflect.ValueOf(object).Elem()
	var tb = tbv.Type()
	var fc = tb.NumField()

	for i := 0; i < fc; i += 1 {
		var fd = tb.Field(i)
		var fbv = tbv.Field(i)
		var name = strings.ToLower(fd.Name)
		if name == table.Key {
			key = fbv.Interface()
		} else {
			var _, ok = table.Fields[name]
			if ok {
				if n != 0 {
					s.WriteString(",")
				}
				s.WriteString(fmt.Sprintf(" %s=?", name))
				fs[n] = fbv.Interface()
				n += 1
			}
		}
	}

	s.WriteString(fmt.Sprintf(" WHERE %s=?", table.Key))

	fs[n] = key

	n += 1

	log.Printf("%s %s\n", s.String(), fs)

	return db.Exec(s.String(), fs[:n]...)
}

func DBInsert(db *sql.DB, table *DBTable, prefix string, object interface{}) (sql.Result, error) {
	var tbname = prefix + table.Name
	var s bytes.Buffer
	var w bytes.Buffer
	var fsc = len(table.Fields)
	var fs = make([]interface{}, fsc)
	var n = 0
	var key reflect.Value

	s.WriteString(fmt.Sprintf("INSERT INTO %s(", tbname))
	w.WriteString(" VALUES (")

	var tbv = reflect.ValueOf(object).Elem()
	var tb = tbv.Type()
	var fc = tb.NumField()

	for i := 0; i < fc; i += 1 {
		var fd = tb.Field(i)
		var fbv = tbv.Field(i)
		var name = strings.ToLower(fd.Name)
		if name == table.Key {
			key = fbv
		} else {
			var _, ok = table.Fields[name]
			if ok {
				if n != 0 {
					s.WriteString(",")
					w.WriteString(",")
				}
				s.WriteString(name)
				w.WriteString("?")
				fs[n] = fbv.Interface()
				n += 1
			}
		}
	}

	s.WriteString(")")

	w.WriteString(")")

	s.Write(w.Bytes())

	log.Printf("%s %s\n", s.String(), fs)

	var rs, err = db.Exec(s.String(), fs[:n]...)

	if err == nil && key.CanSet() {
		id, err := rs.LastInsertId()
		if err == nil {
			key.SetInt(id)
		}
	}

	return rs, err
}

type DBValue struct {
	String  string
	Int64   int64
	Double  float64
	Boolean bool
}

type DBScaner struct {
	object interface{}
	fields []interface{}
}

func NewDBScaner(object interface{}) DBScaner {
	return DBScaner{object, nil}
}

func (o *DBScaner) Scan(rows *sql.Rows) error {

	if o.fields == nil {

		var columns, err = rows.Columns()

		if err != nil {
			return err
		}

		var fdc = len(columns)
		var mi = map[string]int{}

		for i := 0; i < fdc; i += 1 {
			mi[columns[i]] = i
		}

		o.fields = make([]interface{}, fdc)

		var tbv = reflect.ValueOf(o.object).Elem()
		var tb = tbv.Type()
		var fc = tb.NumField()

		for i := 0; i < fc; i += 1 {
			var fd = tb.Field(i)
			var fbv = tbv.Field(i)
			var name = strings.ToLower(fd.Name)
			var idx, ok = mi[name]
			if ok {
				o.fields[idx] = fbv.Addr().Interface()
			}
		}

	}

	return rows.Scan(o.fields...)
}
