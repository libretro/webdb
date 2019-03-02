package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/libretro/ludo/rdb"
)

var target = "build"

// Scrub characters that are not cross-platform and/or violate the
// No-Intro filename standard.
func scrubIllegalChars(str string) string {
	str = strings.Replace(str, "&", "_", -1)
	str = strings.Replace(str, "*", "_", -1)
	str = strings.Replace(str, "/", "_", -1)
	str = strings.Replace(str, ":", "_", -1)
	str = strings.Replace(str, "`", "_", -1)
	str = strings.Replace(str, "<", "_", -1)
	str = strings.Replace(str, ">", "_", -1)
	str = strings.Replace(str, "?", "_", -1)
	str = strings.Replace(str, "|", "_", -1)
	str = strings.Replace(str, "#", "_", -1)
	str = strings.Replace(str, "%", "_", -1)
	return str
}

func LoadDB(dir string) (rdb.DB, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return rdb.DB{}, err
	}
	db := make(rdb.DB)
	for _, f := range files {
		name := f.Name()
		if !strings.Contains(name, ".rdb") {
			continue
		}
		system := name[0 : len(name)-4]
		bytes, _ := ioutil.ReadFile(filepath.Join(dir, name))
		db[system] = rdb.Parse(bytes)
	}
	return db, nil
}

func main() {
	tmpl := template.Must(template.ParseGlob("templates/*"))

	db, err := LoadDB("./database")
	if err != nil {
		log.Fatal(err)
	}

	wg := sync.WaitGroup{}
	for system, games := range db {
		os.MkdirAll(filepath.Join(target, system), os.ModePerm)

		f, err := os.OpenFile(filepath.Join(target, system, "index.html"), os.O_CREATE|os.O_WRONLY, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
		tmpl.ExecuteTemplate(f, "system.html", struct {
			System string
			Games  rdb.RDB
		}{
			system,
			games,
		})
		f.Close()

		wg.Add(1)
		system := system
		games := games
		go func() {
			for _, game := range games {
				cleanName := scrubIllegalChars(game.Name)
				path := filepath.Join(target, system, cleanName+".html")

				f2, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.ModePerm)
				if err != nil {
					log.Fatal(err)
				}

				tmpl.ExecuteTemplate(f2, "game.html", struct {
					System    string
					Game      rdb.Game
					CleanName string
				}{
					system,
					game,
					cleanName,
				})

				f2.Close()
			}
			wg.Done()
		}()
	}
	wg.Wait()
}
