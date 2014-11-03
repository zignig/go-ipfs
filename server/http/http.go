package http

import (
	"fmt"
	"github.com/GeertJohan/go.rice"
	"github.com/gin-gonic/gin"
	"html/template"
	"io/ioutil"
	"net/http"

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
//
//ref https://github.com/spf13/dagobah

// Serve is called from a command path
func Serve(address ma.Multiaddr, node *core.IpfsNode) error {
	log.Critical("starting http server")

	//base router
	r := gin.Default()

	// bind the ipfs service
	handler := &handler{}
	handler.ipfs = &ipfsHandler{node}

	// load the templates
	handler.templ = LoadTemplates("index.html")
	r.SetHTMLTemplate(handler.templ)

	// top level routers

	// ipns router
	r.GET("/ipns/*path", handler.ipnsResolve)

	// ipfs sub router
	r.GET("/ipfs/*path", handler.ipfsResolve)

	// static files subrouter
	r.GET("/static/*filepath", handler.staticFiles)

	//r.HandleFunc("/ipfs/", handler.postHandler).Methods("POST")

	// some template tests
	r.GET("/", handler.templateTest)

	// TODO fix static
	//r.PathPrefix("/ipfs/").Handler(handler).Methods("GET")

	// bind the top level routes
	//http.Handle("/ipfs", ipfsRouter)
	//http.Handle("/ipns", ipnsRouter)
	//http.Handle("/static", staticRouter)

	// TODO admin,api,tour,etc..

	// base router
	http.Handle("/", r)

	_, host, err := manet.DialArgs(address)
	if err != nil {
		return err
	}

	return http.ListenAndServe(host, nil)
}

func (i *handler) ipnsResolve(c *gin.Context) {
	ipnsPath := c.Params.ByName("path")

	log.Error("%s", ipnsPath)
	return
}

func (i *handler) templateTest(c *gin.Context) {
	obj := gin.H{"title": "The Title"}
	c.HTML(200, "index.html", obj)
}

// serve out base static files
// ref http://semantic-ui.com/  , ipfs html fragments ??
func (i *handler) staticFiles(c *gin.Context) {
	static, err := rice.FindBox("static")
	if err != nil {
		log.Fatal(err)
	}
	original := c.Request.URL.Path
	c.Request.URL.Path = c.Params.ByName("filepath")
	http.FileServer(static.HTTPBox()).ServeHTTP(c.Writer, c.Request)
	c.Request.URL.Path = original
}

func (i *handler) ipfsResolve(c *gin.Context) {
	path := c.Params.ByName("path")[1:]
	fmt.Println("handle " + path)

	nd, err := i.ResolvePath(path)
	if err != nil {
		//w.WriteHeader(http.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	var data []byte
	dr, err := i.NewDagReader(nd)
	if err != nil {
		// Return correct data for error type
		log.Critical("%s", err)
		if err == uio.ErrIsDir {
			log.Critical("is directory %s", path)
			if path[len(path)-1:] != "/" {
				log.Critical("missing trailing slash redirect")
				c.Redirect(307, "/ipfs/"+path+"/")
				return
			}
			// loop through files
			//directoryListing := make([]string)
			for _, link := range nd.Links {
				// TODO search for more than index.html ?
				if link.Name == "index.html" {
					log.Info("found index")
					// return index page
					nd, err := i.ResolvePath(path + "/index.html")
					if err != nil {
						log.Error("%s", err)
						return
					}
					dr, err := i.NewDagReader(nd)
					if err != nil {
						log.Error("%s", err)
						return
					}
					// write out the index page
					data, _ = ioutil.ReadAll(dr)
					c.Data(200, "text/html", data)
					return
				}
				//&directoryListing.Append(link)
			}
			// TODO retrun directoryListing and a templated page
		}
		// TODO: return json object containing the tree data if it's a directory (err == ErrIsDir)
		//w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// copy the dag data back to the http request
	data, _ = ioutil.ReadAll(dr)
	c.Data(200, "", data)
	return
}

// out of action for now
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

// Load templates from the rice box.
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
