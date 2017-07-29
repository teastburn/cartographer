// Cartographer is a real time ingestion and mapping system for high loads of geo coordinates.
// The inbound API of coordinates is HTTP/RPC and outbound to a mapping UI is via WebSocket.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"time"
	"crypto/tls"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/labstack/gommon/log"
	"github.com/newrelic/go-agent"
	"github.com/olebedev/emitter"
	"golang.org/x/net/http2"
	"bytes"
)

const (
	channelName = "main:write_loc"
)

var (
	// Echo errors
	ErrStatusHTTPVersionNotSupported = echo.NewHTTPError(http.StatusHTTPVersionNotSupported)

	// Socket config
	socketPingPeriod time.Duration // Send pings to peer with this period. Must be less than pongWait.

	// Flags
	serverPort = flag.Int("p", 8080, "Port to listen on")
	maxConcurrentRequests = flag.Int("c", 1000, "HTTP2 MAX_CONCURRENT_REQUESTS server setting.")
	pprof = flag.Bool("pp", false, "Port to listen on")
	nrLicenseKey = flag.String("n", "", "New Relic License key")
	nrAppSuffix = flag.String("s", "dev", "New Relic suffix for app name")
	socketWriteWait = flag.Duration("sww", 3 * time.Second, " Time allowed to write a message to the peer")
	socketPongWait = flag.Duration("spw", 3 * time.Second, "Time allowed to read the next pong message from the peer")
)

var (
	e = &emitter.Emitter{}
	upgrader = websocket.Upgrader{}
	newline = []byte{'\n'}
	space = []byte{' '}
)

type Geoloc struct {
	Lat float32 `json:"lat"`
	Lon float32 `json:"lon"`
}

func main() {
	flag.Parse()
	socketPingPeriod = (*socketPongWait * 9) / 10 // Send pings to peer with this period. Must be less than pongWait.

	e := echo.New()
	//e.Pre(middleware.HTTPSRedirect())

	go func() {
		log.Fatal(http.ListenAndServeTLS(":6060", "cert.pem", "key.pem", nil))
	}()

	// Middleware
	e.Use(middleware.Recover())
	e.Use(middleware.Secure())
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger())
	e.Logger.SetLevel(log.DEBUG)

	// Custom middleware
	e.Use(middlewareForceHttp2())
	useMiddlewareNewRelic(e, *nrLicenseKey, *nrAppSuffix)

	e.Static("/static", "static")

	e.GET("/sleep", sleepHandler)
	e.GET("/info", infoHandler)
	e.GET("/ws", wsHandler)
	e.POST("/geo", locHandler)

	e.Logger.Fatal(startServer(e, fmt.Sprintf(":%d", *serverPort)))

	//e.Logger.Fatal(e.StartTLS(address, "cert.pem", "key.pem"))
}

func startServer(e *echo.Echo, address string) (err error) {
	// Copied from echo.StartTLS
	s := e.TLSServer
	s.TLSConfig = new(tls.Config)
	s.TLSConfig.Certificates = make([]tls.Certificate, 1)
	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	if err != nil {
		return err
	}
	s.TLSConfig.Certificates[0] = cert
	s.Addr = address
	if !e.DisableHTTP2 {
		s.TLSConfig.NextProtos = append(s.TLSConfig.NextProtos, "h2")
	}
	// Copy end

	// configure our http2 setup
	http2.ConfigureServer(s, &http2.Server{
		MaxConcurrentStreams: 1000,
	})

	return e.StartServer(s)
}

// POST /geo
// Body: {"lat":123.45,"lon":67.890}
// Write coordinate to datastore and publish to listeners
// Only accepts 1 coordinate to simplify logic and still be fast with http2
func locHandler(c echo.Context) error {
	if cc, ok := c.(*CustomContext); ok {
		c = cc
	}
	g := new(Geoloc)
	if err := c.Bind(g); err != nil {
		return err
	}
	e.Emit(channelName, g)
	return nil
}

// Websocket /ws
// Opens a web socket listener
func wsHandler(c echo.Context) error {
	if cc, ok := c.(*CustomContext); ok {
		c = cc
	}
	c.Logger().Debug("wsHandler open")
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	ws.SetCloseHandler(func(code int, text string) error {
		c.Logger().Debugf("wsHandler close. received message. %d, %s", code, text)
		return nil
	})
	//ws.SetWriteDeadline(time.Now().Add(*socketWriteWait))

	go readPump(c, ws)
	go writePump(c, ws)

	return nil
}

func infoHandler(c echo.Context) error {
	if cc, ok := c.(*CustomContext); ok {
		c = cc
	}
	req := c.Request()
	reqInfo := map[string]interface{}{
		"protocol":                 req.Proto,
		"listeners":                len(e.Listeners(channelName)),
		"goroutines":               runtime.NumGoroutine(),
		"numCPU":                   runtime.NumCPU(),
		"concurrentRequestsServer": *maxConcurrentRequests,
	}
	return c.JSON(http.StatusOK, reqInfo)
}

func sleepHandler(c echo.Context) error {
	time.Sleep(300 * time.Second)
	return c.JSON(http.StatusOK, nil)
}

// Force clients to use HTTP 2, except with websockets
func middlewareForceHttp2() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			isAtLeast2 := c.Request().ProtoAtLeast(2, 0)
			upgreadHeader := c.Request().Header.Get("Upgrade")
			isWebsocket := upgreadHeader != "" && upgreadHeader == "websocket"
			if isAtLeast2 || isWebsocket {
				return next(c)
			}
			c.Logger().Errorf("Version not supported: %s", c.Request().Proto)
			return ErrStatusHTTPVersionNotSupported
		}
	}
}

type CustomContext struct {
	app newrelic.Application
	echo.Context
}

func useMiddlewareNewRelic(e *echo.Echo, nrLicenseKey string, nrAppSuffix string) {
	if nrLicenseKey != "" {
		log.Printf("Using NR %s", nrLicenseKey)
		config := newrelic.NewConfig(fmt.Sprintf("cartographer-%s", nrAppSuffix), nrLicenseKey)
		app, err := newrelic.NewApplication(config)
		if err != nil {
			log.Printf("Failed to setup NR. Skipping. %v", err)
			return
		}
		e.Use(middlewareNewRelic(app))
		return
	}
	log.Printf("No NR config. Skipping. %#v", nrLicenseKey)
}

func middlewareNewRelic(app newrelic.Application) echo.MiddlewareFunc {
	return func(h echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			txn := app.StartTransaction(c.Request().URL.Path, c.Response(), c.Request())
			defer txn.End()
			cc := &CustomContext{
				app,
				c,
			}
			return h(cc)
		}
	}
}

func writePump(c echo.Context, ws *websocket.Conn) {
	ch := e.On(channelName)
	ticker := time.NewTicker(socketPingPeriod)

	defer func() {
		e.Off(channelName, ch)
		ticker.Stop()
		ws.Close()
	}()

	for {
		select {
		case event := <-ch:
			if loc, ok := event.Args[0].(*Geoloc); ok {
				err := ws.WriteJSON(loc)
				if err != nil {
					if err, ok := err.(*net.OpError); ok {
						c.Logger().Debugf("wsHandler close. error writing %s. closing\n", err)
						return
					}
					c.Logger().Warnf("error writing %s. continuing\n", err)
				}
			} else {
				c.Logger().Errorf("failed to parse loc %#v\n", event.Args[0])
			}
		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(*socketWriteWait)); err != nil {
				c.Logger().Debugf("wsHandler close. Ping failed. err: %v", err)
				return
			}
		}
	}
}

func readPump(c echo.Context, ws *websocket.Conn) {
	defer func() {
		ws.Close()
	}()
	ws.SetPongHandler(func(s string) error {
		ws.SetReadDeadline(time.Now().Add(*socketPongWait));
		return nil
	})
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				c.Logger().Errorf("Unexpected ws conn error: %v\n", err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
	}
}
