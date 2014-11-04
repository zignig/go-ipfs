package http

import "github.com/gin-gonic/gin"

func (i *handler) showApi(c *gin.Context) {
	// api goes here

	data := i.renderContent("api.html", nil)
	obj := gin.H{"title": "Api V1", "data": data, "menu": i.menu, "section": "api"}
	c.HTML(200, "index.html", obj)
}
