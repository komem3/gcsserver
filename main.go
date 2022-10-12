package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"

	"google.golang.org/api/iterator"
)

type htmlProp struct {
	Base        string
	Directories []string
}

var targetBuckets = strings.Split(os.Getenv("BUCKET"), ",")

var htmlFile = `
<html>
  <body>
    <h1>{{.Base}} Directory</h1>
    <ul>
      {{range .Directories}}
      <li><a href="{{.}}">{{.}}</a></li>
      {{end}}
    </ul>
    <a href="/">Back ←</a>
  </body>
</html>
`

var templateHTML = template.Must(template.New("index.html").Parse(htmlFile))

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	for _, target := range targetBuckets {
		http.Handle("/"+target+"/", http.StripPrefix("/"+target, FileServe(ctx, client, target)))
	}

	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		if err := templateHTML.Execute(w, htmlProp{
			Base:        "/",
			Directories: targetBuckets,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	panic(http.ListenAndServe(":"+os.Getenv("PORT"), nil))
}

func FileServe(ctx context.Context, client *storage.Client, bucket string) http.Handler {
	mux := http.NewServeMux()

	iter := client.Bucket(bucket).Objects(ctx, nil)
	var paths []string
	for {
		obj, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(fmt.Errorf("bucket %s: %w", bucket, err))
		}
		serveFile := func(w http.ResponseWriter, _ *http.Request) {
			f, err := client.Bucket(bucket).Object(obj.Name).NewReader(ctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer f.Close()

			if _, err := io.Copy(w, f); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		url, err := url.Parse("/" + obj.Name)
		if err != nil {
			panic(err)
		}
		mux.HandleFunc(url.EscapedPath(), serveFile)

		paths = append(paths, filepath.Join("/", bucket, obj.Name))
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		if err := templateHTML.Execute(w, htmlProp{
			Base:        "/" + bucket,
			Directories: paths,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	return mux
}
