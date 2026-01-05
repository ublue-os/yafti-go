package server

import (
	"context"
	"embed"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/Zeglius/yafti-go/config"
	"github.com/Zeglius/yafti-go/internal/consts"
	"github.com/Zeglius/yafti-go/ui/pages"
	"github.com/a-h/templ"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Server struct {
	e            *echo.Echo
	lastBeat     time.Time
	m            sync.Mutex
	shutdownCtx  context.Context
	cancel       context.CancelFunc
	StaticAssets *embed.FS // This var is set in main.go
}

// Default [templ.Handler] with streaming enabled by default
func newHandler(c templ.Component, options ...func(*templ.ComponentHandler)) *templ.ComponentHandler {
	opts := []func(*templ.ComponentHandler){templ.WithStreaming()}
	opts = append(opts, options...)
	return templ.Handler(c, opts...)
}

func New() *Server {
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		e:           e,
		lastBeat:    time.Now(),
		shutdownCtx: ctx,
		cancel:      cancel,
	}
}

func (s *Server) heartbeatHandler(c echo.Context) error {
	s.m.Lock()
	s.lastBeat = time.Now()
	s.m.Unlock()
	return c.String(http.StatusOK, "Hearthbeat received")
}

func (s *Server) monitorHeartbeat() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdownCtx.Done():
			return
		case <-ticker.C:
			s.m.Lock()
			if time.Since(s.lastBeat) > consts.HEARTBEAT_SECONDS*time.Second && config.Inhibit.Load() == 0 {
				log.Printf("No heartbeat for %d seconds, shutting down server\n", consts.HEARTBEAT_SECONDS)
				s.m.Unlock()
				s.cancel()
				if err := s.e.Shutdown(context.Background()); err != nil {
					log.Printf("Shutdown error: %v", err)
				}
				return
			}
			s.m.Unlock()
		}
	}
}

func (s *Server) Start() error {
	e := s.e

	e.Use(middleware.Logger())

	// Set up static file serving
	if s.StaticAssets == nil {
		return errors.New("StaticAssets is not populated. Ensure it is set in main.go")
	}
	fs := echo.MustSubFS(*s.StaticAssets, "static")
	e.StaticFS("/static/", fs)

	// Handle heartbeat, so we shutdown the server automatically
	// when there is no client connected over a period of time.
	e.GET("/_/heartbeat", s.heartbeatHandler)

	// Handle pages routes
	e.GET("/", echo.WrapHandler(
		newHandler(pages.Home()),
	))

	e.GET("/about", func(c echo.Context) error {
		return c.NoContent(http.StatusNotFound)
	})

	e.GET("/_/dummy", func(c echo.Context) error {
		return c.JSON(http.StatusOK, struct {
			Name    string   `json:"name"`
			Age     int      `json:"age"`
			Hobbies []string `json:"hobbies"`
		}{
			Name:    "John Doe",
			Age:     30,
			Hobbies: []string{"Reading", "Hiking", "Cooking"},
		})
	})

	e.GET("/action_group/:idx", func(c echo.Context) error {
		var screen config.Screen

		sId, err := strconv.Atoi(c.Param("idx"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid screen index")
		}

		if sId < 0 || sId >= len(config.ConfStatus.Screens) {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid screen index")
		}
		screen = config.ConfStatus.Screens[sId]

		handler := newHandler(pages.ActionGroupScreen(screen))
		handler.ServeHTTP(c.Response(), c.Request())

		return nil
	})

	e.POST("/_/execute/:actionId", func(c echo.Context) error {
		actionID := c.Param("actionId")
		
		// Find the action by ID
		var action config.Action
		found := false
		for act := range config.ConfStatus.GetAllActions() {
			if act.ID == actionID {
				action = act
				found = true
				break
			}
		}

		if !found {
			return echo.NewHTTPError(http.StatusNotFound, "Action not found")
		}

		if action.Script == "" {
			return c.String(http.StatusBadRequest, "Action has no script to execute")
		}

		// Launch command inline (websocket PTY)
		handler := newHandler(pages.LaunchCommandInTerminal(actionID, action.Title, action.Script))
		handler.ServeHTTP(c.Response(), c.Request())

		return nil
	})

	// WebSocket endpoint to run a command inside a PTY and stream I/O
	e.GET("/_/ws/exec/:actionId", func(c echo.Context) error {
		actionID := c.Param("actionId")

		// Find the action by ID
		var action config.Action
		found := false
		for act := range config.ConfStatus.GetAllActions() {
			if act.ID == actionID {
				action = act
				found = true
				break
			}
		}

		if !found {
			return echo.NewHTTPError(http.StatusNotFound, "Action not found")
		}

		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}

		// Start the command in a PTY so interactive programs work
		cmd := exec.Command("bash", "-c", action.Script)
		ptmx, err := pty.Start(cmd)
		if err != nil {
			ws.WriteMessage(websocket.TextMessage, []byte("Error starting command: "+err.Error()))
			ws.Close()
			return nil
		}

		// PTY -> WebSocket
		go func() {
			defer func() {
				ptmx.Close()
				ws.Close()
			}()
			buf := make([]byte, 1024)
			for {
				n, err := ptmx.Read(buf)
				if n > 0 {
					// Send as binary so arbitrary bytes are preserved
					if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()

		// WebSocket -> PTY
		for {
			mt, message, err := ws.ReadMessage()
			if err != nil {
				// client closed
				cmd.Process.Kill()
				ptmx.Close()
				break
			}
			if mt == websocket.TextMessage || mt == websocket.BinaryMessage {
				_, _ = ptmx.Write(message)
			}
		}

		return nil
	})

	e.POST("/_/post_test", func(c echo.Context) error {
		data := struct {
			POSTParams url.Values        `json:"POST_params"`
			Cookies    map[string]string `json:"cookies"`
		}{}

		if v, err := c.FormParams(); err != nil {
			return c.JSON(http.StatusInternalServerError, err)
		} else {
			data.POSTParams = v
		}

		if c.Cookies() != nil {
			data.Cookies = make(map[string]string)
			for _, v := range c.Cookies() {
				k := v.Name
				data.Cookies[k] = v.Value
			}
		}

		return c.JSON(http.StatusOK, data)
	})

	// Start server
	go s.monitorHeartbeat()
	log.Printf("Server started at http://localhost:%s", consts.PORT)
	return s.e.Start("127.0.0.1:" + consts.PORT)
}
