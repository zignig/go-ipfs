package http

import "github.com/gin-gonic/gin"

func (i *handler) showMap(c *gin.Context) {
	// Show peer maps
	data := i.renderContent("map.html", nil)
	obj := gin.H{"title": "Network Map", "data": data, "menu": i.menu, "section": "map"}
	c.HTML(200, "index.html", obj)
}
