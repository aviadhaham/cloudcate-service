package web

import (
	"log"
	"net/http"

	config "github.com/aviadhaham/cloudcate/internal/aws_search/config"
	search "github.com/aviadhaham/cloudcate/internal/aws_search/search"

	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
)

func NewRouter(profiles []string) *gin.Engine {
	r := gin.Default()

	// Serve react app
	r.Use(static.Serve("/", static.LocalFile("./web/dist", true)))

	api := r.Group("/api")
	{
		api.GET("/search", func(c *gin.Context) {
			resourceName := c.Query("resource_name")
			resourceType := c.Query("resource_type")
			resourceSubType := c.Query("resource_subtype")

			results, err := search.FindResources(profiles, config.ServicesGlobality, resourceType, resourceSubType, resourceName)
			if err != nil {
				log.Fatalf("Failed to search resources: %v", err)
			}

			c.JSON(http.StatusOK, gin.H{
				"results": results,
			})
		})
	}

	return r
}
