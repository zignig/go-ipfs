package http

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/GeertJohan/go.rice"
	"github.com/gin-gonic/gin"

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
	menu  *Menu
}

var log = u.Logger("http")

// Create menu , should this be per context or global ?
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

	// setup menu
	handler.menu = NewMenu("main")
	handler.menu.AddItem("tour", "tour/", "suitcase")
	r.GET("/tour/*path", handler.runTour)

	handler.menu.AddItem("map", "map/", "sitemap")

	handler.menu.AddItem("settings", "settings/", "settings")

	handler.menu.AddItem("peers", "peers/", "list")
	// load the templates
	handler.templ = LoadTemplates("tour.html", "landing.html", "menu.html", "index.html")
	r.SetHTMLTemplate(handler.templ)

	// top level routers

	// ipns router
	r.GET("/ipns/*path", handler.ipnsResolve)

	// ipfs sub router
	r.GET("/ipfs/*path", handler.ipfsResolve)

	// static files subrouter
	r.GET("/static/*filepath", handler.staticFiles)

	//r.HandleFunc("/ipfs/", handler.postHandler).Methods("POST")

	// Landing Page
	r.GET("/", handler.landingPage)

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

func (i *handler) landingPage(c *gin.Context) {
	data := i.renderContent("landing.html", nil)
	obj := gin.H{"title": "The Title", "data": data, "menu": i.menu, "section": ""}
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
		c.String(500, "%s", err)
		fmt.Println(err)
		return
	}

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
						c.String(500, "%s", err)
						log.Error("%s", err)
						return
					}
					dr, err := i.NewDagReader(nd)
					if err != nil {
						c.String(500, "%s", err)
						log.Error("%s", err)
						return
					}
					// write out the index page
					var data []byte
					data, _ = ioutil.ReadAll(dr)
					c.Data(200, "text/html", data)
					return
				}
				//&directoryListing.Append(link)
			}
			// TODO return directoryListing and a templated page
		}
		// TODO: return json object containing the tree data if it's a directory (err == ErrIsDir)
		//w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// copy the dag data back to the http request
	io.Copy(c.Writer, dr)
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

// Page content
// Probably not the best , need to look at define in templates
func (i *handler) renderContent(name string, data interface{}) template.HTML {
	var doc bytes.Buffer
	i.templ.ExecuteTemplate(&doc, name, data)
	s := doc.String()
	return template.HTML(s)
}

// Load templates from the rice box.
func LoadTemplates(list ...string) *template.Template {
	templateBox, err := rice.FindBox("templates")
	if err != nil {
		log.Critical("%s", err)
	}
	templates := template.New("")

	// some helpers
	funcMap := template.FuncMap{
		"safehtml": func(text string) template.HTML { return template.HTML(text) },
	}

	templates.Funcs(funcMap)
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
