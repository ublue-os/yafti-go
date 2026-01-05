package server

import (
	"context"
	"embed"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/Zeglius/yafti-go/config"
	"github.com/Zeglius/yafti-go/internal/consts"
	"github.com/Zeglius/yafti-go/ui/pages"
	"github.com/a-h/templ"
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
			if time.Since(s.lastBeat) > consts.HEARTBEAT_SECONDS*time.Second && !config.Inhibit.Load() {
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

	e.GET("/apply_changes/:id", func(c echo.Context) error {
		config.Inhibit.Store(true)
		defer config.Inhibit.Store(false)

		actionId := c.Param("id")
		if actionId == "" {
			return c.String(http.StatusBadRequest, "missing action_id")
		}

		// Execute the action with the given ID
		ExecuteActionWithId(actionId)

		// Return to /
		return c.Redirect(http.StatusFound, "/")
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
