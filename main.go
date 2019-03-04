package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/libretro/ludo/rdb"
)

var target = "build"
var tmpl *template.Template

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

func loadDB(dir string) (rdb.DB, error) {
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

func buildHome(db rdb.DB) {
	os.MkdirAll(target, os.ModePerm)
	os.Link("img-broken.png", filepath.Join(target, "img-broken.png"))

	f, err := os.OpenFile(filepath.Join(target, "index.html"), os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	tmpl.ExecuteTemplate(f, "home.html", struct {
		DB rdb.DB
	}{
		db,
	})
}

func buildSystem(system string, games rdb.RDB) {
	os.MkdirAll(filepath.Join(target, system), os.ModePerm)

	f, err := os.OpenFile(filepath.Join(target, system, "index.html"), os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	tmpl.ExecuteTemplate(f, "system.html", struct {
		System string
		Games  rdb.RDB
	}{
		system,
		games,
	})
}

func buildGame(system string, game rdb.Game) {
	cleanName := scrubIllegalChars(game.Name)
	path := filepath.Join(target, system, cleanName+".html")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	tmpl.ExecuteTemplate(f, "game.html", struct {
		System    string
		Game      rdb.Game
		CleanName string
	}{
		system,
		game,
		cleanName,
	})
}

var funcMap = template.FuncMap{
	"N": func(n int) []struct{} {
		return make([]struct{}, n)
	},
}

func build() {
	tmpl = template.Must(
		template.New("main").Funcs(funcMap).ParseGlob("templates/*.html"),
	)

	db, err := loadDB("./database")
	if err != nil {
		log.Fatal(err)
	}

	buildHome(db)

	wg := sync.WaitGroup{}
	for system, games := range db {
		buildSystem(system, games)
		system := system
		games := games
		wg.Add(1)
		go func() {
			for _, game := range games {
				buildGame(system, game)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func serve() {
	fs := http.FileServer(http.Dir(target))
	http.Handle("/", fs)

	log.Println("Listening on http://0.0.0.0:3003")
	http.ListenAndServe(":3003", nil)
}

func main() {
	flag.Parse()
	args := flag.Args()
	switch args[0] {
	case "serve":
		serve()
	case "build":
		fallthrough
	default:
		build()
	}
}
