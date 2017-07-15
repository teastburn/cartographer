package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/labstack/gommon/log"
	"github.com/olebedev/emitter"
)

const channelName = "main:write_loc"

// config options (cli)
var (
	serverPort = flag.Int("p", 8080, "Port to listen on")
	dbHost     = flag.String("d", "cassandra", "Cx host")
	pointTTL   = flag.Int("ttl", 36000, "TTL in seconds of the life of a row")
)

var (
	e        = &emitter.Emitter{}
	upgrader = websocket.Upgrader{}
)

type Geoloc struct {
	Lat float32 `json:"lat"`
	Lon float32 `json:"lon"`
}

func wsHandler(c echo.Context) error {
	c.Logger().Debug("wsHandler open")
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	ch := e.On(channelName)
	defer e.Off(channelName, ch)
	for event := range ch {
		c.Logger().Debugf("received %#v", event)
		if loc, ok := event.Args[0].(*Geoloc); ok {
			c.Logger().Debugf("received and reemitting %#v\n", loc)
			err = ws.WriteJSON(loc)
			if err != nil {
				if err, ok := err.(*net.OpError); ok {
					c.Logger().Debugf("error writing %s. closing\n", err)
					break
				}
				c.Logger().Warnf("error writing %s. continuing\n", err)
			}
		} else {
			c.Logger().Errorf("failed to parse loc %#v\n", event.Args[0])
		}
	}

	c.Logger().Debug("wsHandler close")
	return nil
}

func infoHandler(c echo.Context) error {
	req := c.Request()
	reqInfo := map[string]interface{}{
		"protocol": req.Proto,
		"listeners": len(e.Listeners(channelName)),
	}
	return c.JSON(http.StatusOK, reqInfo)
}

func locHandler(c echo.Context) error {
	g := new(Geoloc)
	if err := c.Bind(g); err != nil {
		return err
	}
	c.Logger().Debugf("start writing %#v", g)
	e.Emit(channelName, g)
	c.Logger().Debugf("done writing %#v", g)
	return nil
}

func main() {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Logger.SetLevel(log.DEBUG)

	err := dbInit(*dbHost)
	if err != nil {
		e.Logger.Fatalf("Failed to connect to database: %v", err)
	}

	e.Static("/static", "static")

	e.GET("/info", infoHandler)
	e.GET("/ws", wsHandler)
	e.POST("/geo", locHandler)

	port := fmt.Sprintf(":%d", *serverPort)
	e.Logger.Fatal(e.StartTLS(port, "cert.pem", "key.pem"))
}
