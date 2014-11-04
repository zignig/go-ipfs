package http

import "github.com/gin-gonic/gin"

func (i *handler) runTour(c *gin.Context) {
	data := i.renderContent("tour.html", nil)
	obj := gin.H{"title": "The Title", "data": data, "menu": i.menu, "section": "tour"}
	c.HTML(200, "index.html", obj)
}
