package http

import "github.com/gin-gonic/gin"

func (i *handler) showPeers(c *gin.Context) {
	// fill in the settings editor
	data := i.renderContent("peers.html", nil)
	obj := gin.H{"title": "Peers", "data": data, "menu": i.menu, "section": "peers"}
	c.HTML(200, "index.html", obj)
}
