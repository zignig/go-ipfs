package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/gorilla/mux"
	ma "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multiaddr"
	manet "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multiaddr/net"
	mh "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multihash"

	cmds "github.com/jbenet/go-ipfs/commands"
	core "github.com/jbenet/go-ipfs/core"
	"github.com/jbenet/go-ipfs/core/commands"
)

type objectHandler struct {
	ipfs
}

type apiHandler struct{}

// Serve starts the http server
func Serve(address ma.Multiaddr, node *core.IpfsNode) error {
	r := mux.NewRouter()
	objectHandler := &objectHandler{&ipfsHandler{node}}
	apiHandler := &apiHandler{}

	r.PathPrefix("/api/v0/").Handler(apiHandler).Methods("GET", "POST")

	r.HandleFunc("/ipfs/", objectHandler.postHandler).Methods("POST")
	r.PathPrefix("/ipfs/").Handler(objectHandler).Methods("GET")

	http.Handle("/", r)

	_, host, err := manet.DialArgs(address)
	if err != nil {
		return err
	}

	return http.ListenAndServe(host, nil)
}

func (i *objectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[5:]

	nd, err := i.ResolvePath(path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Println(err)
		return
	}

	dr, err := i.NewDagReader(nd)
	if err != nil {
		// TODO: return json object containing the tree data if it's a directory (err == ErrIsDir)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Println(err)
		return
	}

	io.Copy(w, dr)
}

func (i *objectHandler) postHandler(w http.ResponseWriter, r *http.Request) {
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

func (i *apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.Split(r.URL.Path, "/")[3:]
	opts := getOptions(r)

	// TODO: get args

	// ensure the requested command exists, otherwise 404
	_, err := commands.Root.Get(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404 page not found"))
		return
	}

	// build the Request and call the command
	req := cmds.NewRequest(path, opts, nil, nil)
	res := commands.Root.Call(req)

	// if response contains an error, write an HTTP error status code
	if err = res.Error(); err != nil {
		e := err.(cmds.Error)

		if e.Code == cmds.ErrClient {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}

	val := res.Value()

	// if the output value is a io.Reader, stream its output in the request body
	if stream, ok := val.(io.Reader); ok {
		io.Copy(w, stream)
		return
	}

	// otherwise, marshall and output the response value or error
	if val != nil || res.Error() != nil {
		output, err := res.Marshal()

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Println(err)
			return
		}

		if output != nil {
			w.Write(output)
		}
	}
}

// getOptions returns the command options in the given HTTP request
// (from the querystring and request body)
func getOptions(r *http.Request) map[string]interface{} {
	opts := make(map[string]interface{})

	query := r.URL.Query()
	for k, v := range query {
		opts[k] = v[0]
	}

	// TODO: get more options from request body (formdata, json, etc)

	if _, exists := opts[cmds.EncShort]; !exists {
		opts[cmds.EncShort] = cmds.JSON
	}

	return opts
}
