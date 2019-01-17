package main

import (
	"archive/zip"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultDBFile = "drive_stats.db"
)

var (
	dbFile  string
	wipe    bool
	driveDB *db
)

func main() {
	var err error

	flag.StringVar(&dbFile, "db", defaultDBFile, "Database file")
	flag.BoolVar(&wipe, "clean", false, "Delete old database before starting")
	flag.Parse()

	if wipe {
		removeFile(dbFile)
		removeFile(dbFile + "-journal")
		removeFile(dbFile + "-wal")
	}

	driveDB, err = newDB(dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer driveDB.close()

	for _, dir := range flag.Args() {
		readOneDir(dir)
	}

	err = driveDB.finishLoad()
	if err != nil {
		log.Fatal(err)
	}
}

func removeFile(fname string) {
	_, err := os.Stat(fname)
	if err == nil {
		err = os.Remove(fname)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func readOneDir(dir string) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, inErr error) (outErr error) {
		switch {
		case info == nil:
			return

		case info.IsDir() && info.Name() == "__MACOSX":
			return filepath.SkipDir

		case !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".csv"):
			err := readOneFile(path)
			if err != nil {
				log.Println(err)
			}

		case !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".zip"):
			err := readZipFile(path)
			if err != nil {
				log.Println(err)
			}
		}
		return
	})
}

func readOneFile(fname string) (err error) {
	log.Println(fname)
	s, err := readFile(fname)
	if err != nil {
		return
	}

	err = insertSmartFile(s)

	return
}

func insertSmartFile(s *smartFile) (err error) {
	ins, err := driveDB.prepare(s.columns)
	for _, row := range s.rows {
		err = ins.putRow(row)
		if err != nil {
			break
		}
	}

	if err == nil {
		err = ins.commit()
	} else {
		_ = ins.rollback()
	}

	return
}

func readZipFile(fname string) (err error) {
	r, err := zip.OpenReader(fname)
	if err != nil {
		return
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "__MACOSX") ||
			!strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			continue
		}
		log.Println(f.Name)

		var data io.ReadCloser
		data, err = f.Open()
		if err != nil {
			log.Println(err)
			continue
		}

		var s *smartFile
		s, err = readReader(data)
		data.Close()
		if err != nil {
			log.Println(err)
			continue
		}

		err = insertSmartFile(s)
		if err != nil {
			log.Println(err)
			continue
		}
	}

	return
}
