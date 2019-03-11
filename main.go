package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"text/template"

	"github.com/libretro/ludo/rdb"
)

var target = "build"
var perPage = 24
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
	str = strings.Replace(str, "\"", "_", -1)
	return str
}

func reverse(s rdb.RDB) rdb.RDB {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
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
		db[system] = reverse(rdb.Parse(bytes))
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

func buildSystemPages(system string, games rdb.RDB) {
	os.MkdirAll(filepath.Join(target, system), os.ModePerm)
	numPages := int(math.Ceil(float64(len(games)) / float64(perPage)))
	for p := 0; p < numPages; p++ {
		page := fmt.Sprintf("%d", p)
		f, err := os.OpenFile(filepath.Join(target, system, "index-"+page+".html"), os.O_CREATE|os.O_WRONLY, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}

		tmpl.ExecuteTemplate(f, "systempage.html", struct {
			System   string
			Games    rdb.RDB
			Page     int
			LastPage int
		}{
			system,
			games[p*perPage : min(p*perPage+perPage, len(games))],
			p,
			numPages - 1,
		})

		f.Close()
	}
}

func buildGame(system string, game rdb.Game) {
	path := filepath.Join(target, system, scrubIllegalChars(game.Name)+".html")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	tmpl.ExecuteTemplate(f, "game.html", struct {
		System string
		Game   rdb.Game
	}{
		system,
		game,
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var funcMap = template.FuncMap{
	"mkslice": func(a []rdb.Game, start, end int) []rdb.Game {
		e := min(end, len(a))
		return a[start:e]
	},
	"Clean": scrubIllegalChars,
	"Tags": func(name string) []string {
		_, tags := extractTags(name)
		return tags
	},
	"WithoutTags": func(name string) string {
		sname, _ := extractTags(name)
		return sname
	},
	"add": func(a, b int) int {
		return a + b
	},
	"title": func(name string) string {
		return strings.Title(name)
	},
}

func extractTags(name string) (string, []string) {
	re := regexp.MustCompile(`\(.*?\)`)
	pars := re.FindAllString(name, -1)
	var tags []string
	for _, par := range pars {
		name = strings.Replace(name, par, "", -1)
		par = strings.Replace(par, "(", "", -1)
		par = strings.Replace(par, ")", "", -1)
		results := strings.Split(par, ",")
		for _, result := range results {
			tags = append(tags, strings.TrimSpace(result))
		}
	}
	name = strings.TrimSpace(name)
	return name, tags
}

func build() {
	tmpl = template.Must(
		template.New("main").Funcs(funcMap).ParseGlob("templates/*.html"),
	)

	db, err := loadDB("./database/rdb")
	if err != nil {
		log.Fatal(err)
	}

	buildHome(db)

	wg := sync.WaitGroup{}
	for system, games := range db {
		buildSystemPages(system, games)
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

	buildTags(db, "franchise")
	buildTags(db, "developer")
	buildTags(db, "publisher")
	buildTags(db, "genre")
	buildTags(db, "origin")

	wg.Wait()
}

// Given a metatag like franchise or genre or developer, will create a hierarchy
// of tag files and indexes allowing to browse the database based on this
// property.
func buildTags(db rdb.DB, tagType string) {
	perTag := map[string][]rdb.Game{}
	for system, games := range db {
		for _, game := range games {
			game.System = system
			switch tagType {
			case "franchise":
				if game.Franchise == "" {
					continue
				}
				perTag[game.Franchise] = append(perTag[game.Franchise], game)
			case "developer":
				if game.Developer == "" {
					continue
				}
				perTag[game.Developer] = append(perTag[game.Developer], game)
			case "publisher":
				if game.Publisher == "" {
					continue
				}
				perTag[game.Publisher] = append(perTag[game.Publisher], game)
			case "genre":
				if game.Genre == "" {
					continue
				}
				perTag[game.Genre] = append(perTag[game.Genre], game)
			case "origin":
				if game.Origin == "" {
					continue
				}
				perTag[game.Origin] = append(perTag[game.Origin], game)
			}
		}
	}
	buildTagIndex(perTag, tagType)
	for tag, games := range perTag {
		buildTagPage(tagType, tag, games)
	}
}

// Will build a page like /franchise.html that will list all the franchises in
// the database.
func buildTagIndex(perTag map[string][]rdb.Game, tagType string) {
	os.MkdirAll(filepath.Join(target, tagType), os.ModePerm)
	path := filepath.Join(target, tagType, "index.html")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	err = tmpl.ExecuteTemplate(f, "tags.html", struct {
		TagType string
		PerTag  map[string][]rdb.Game
	}{
		tagType,
		perTag,
	})
	if err != nil {
		log.Fatal(err)
	}
}

// Will build a page like /franchise/Bomberman.html that will list all the
// games of the bomberman franchise.
func buildTagPage(tagType, tag string, games []rdb.Game) {
	path := filepath.Join(target, tagType, scrubIllegalChars(tag)+".html")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	err = tmpl.ExecuteTemplate(f, "tag.html", struct {
		TagType string
		Tag     string
		Games   []rdb.Game
	}{
		tagType,
		tag,
		games,
	})
	if err != nil {
		log.Fatal(err)
	}
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
