package http

import "github.com/gin-gonic/gin"

func (i *handler) showSettings(c *gin.Context) {
	// fill in the settings editor
	data := i.renderContent("settings.html", nil)
	obj := gin.H{"title": "Settings", "data": data, "menu": i.menu, "section": "settings"}
	c.HTML(200, "index.html", obj)
}
