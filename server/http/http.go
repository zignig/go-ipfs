package http

import (
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/GeertJohan/go.rice"
	"github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/gorilla/mux"

	ma "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multiaddr"
	manet "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multiaddr/net"
	mh "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multihash"
	core "github.com/jbenet/go-ipfs/core"
	uio "github.com/jbenet/go-ipfs/unixfs/io"
	u "github.com/jbenet/go-ipfs/util"
)

type handler struct {
	ipfs
	templ *template.Template
}

var log = u.Logger("http")

// Serve starts the http server
func Serve(address ma.Multiaddr, node *core.IpfsNode) error {
	log.Critical("starting http server")
	r := mux.NewRouter()
	handler := &handler{}
	handler.ipfs = &ipfsHandler{node}
	handler.templ = LoadTemplates("index.html")

	r.HandleFunc("/ipfs/", handler.postHandler).Methods("POST")
	r.HandleFunc("/template/", handler.templateTest).Methods("GET")
	r.PathPrefix("/ipfs/").Handler(handler).Methods("GET")
	http.Handle("/", r)

	_, host, err := manet.DialArgs(address)
	if err != nil {
		return err
	}

	return http.ListenAndServe(host, nil)
}

func (i *handler) templateTest(w http.ResponseWriter, r *http.Request) {
	err := i.templ.ExecuteTemplate(w, "index.html", new(interface{}))
	if err != nil {
		log.Critical(" template error %s", err)
	}
}

func (i *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[5:]

	nd, err := i.ResolvePath(path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Println(err)
		return
	}

	dr, err := i.NewDagReader(nd)
	if err != nil {
		if err == uio.ErrIsDir {
			log.Critical("is directory %s", path)
			if path[len(path)-1:] != "/" {
				log.Critical(" missing trailing slash redirect")
				http.Redirect(w, r, "/ipfs/"+path+"/", 307)
				return
			}
			// loop through files
			for _, link := range nd.Links {
				if link.Name == "index.html" {
					log.Info("found index")
					// return index page
					nd, err := i.ResolvePath(path + "/index.html")
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						log.Error("%s", err)
						return
					}
					dr, err := i.NewDagReader(nd)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						log.Error("%s", err)
						return
					}
					io.Copy(w, dr)
					return
				}
			}
		}
		// TODO: return json object containing the tree data if it's a directory (err == ErrIsDir)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	io.Copy(w, dr)
}

func (i *handler) postHandler(w http.ResponseWriter, r *http.Request) {
	nd, err := i.NewDagFromReader(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Println(err)
		return
	}

	k, err := i.AddNodeToDAG(nd)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Println(err)
		return
	}

	//TODO: return json representation of list instead
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(mh.Multihash(k).B58String()))
}

func LoadTemplates(list ...string) *template.Template {
	templateBox, err := rice.FindBox("templates")
	if err != nil {
		log.Critical("%s", err)
	}
	fmt.Println(templateBox)
	templates := template.New("")
	for _, x := range list {
		templateString, err := templateBox.String(x)
		if err != nil {
			log.Fatal(err)
		}
		_, err = templates.New(x).Parse(templateString)
		if err != nil {
			log.Fatal(err)
		}
	}
	return templates
}
