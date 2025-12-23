package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AddParams struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type UserSession struct {
	id   string
	data map[string]interface{}
	mu   sync.RWMutex
}

// Tool handler

func (s *UserSession) SessionID() string {
	return s.id
}

func (s *UserSession) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[key]
	return val, ok
}

func (s *UserSession) Set(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func Add(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params AddParams

	// Marshal raw params into JSON and unmarshal into typed struct
	if req.Params.Arguments == nil {
		return nil, fmt.Errorf("no arguments provided")
	}
	data := req.Params.Arguments
	if err := json.Unmarshal(data, &params); err != nil {
		return nil, fmt.Errorf("cannot unmarshal arguments: %w", err)
	}

	sum := params.X + params.Y

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("%d", sum)},
		},
	}, nil
}

var sessions sync.Map

func getSession(r *http.Request) *UserSession {
	id := r.Header.Get("X-Client-ID")
	if id == "" {
		id = r.RemoteAddr
	}

	session, _ := sessions.LoadOrStore(id, &UserSession{
		id:   id,
		data: make(map[string]interface{}),
	})
	return session.(*UserSession)
}

func main() {
	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "server",
		Version: "v0.0.1",
	}, nil)

	// Add the "add" tool
	server.AddTool(&mcp.Tool{
		Name:        "add",
		Description: "Add two integers",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x": map[string]any{"type": "integer"},
				"y": map[string]any{"type": "integer"},
			},
			"required": []string{"x", "y"},
		},
	}, Add)

	handler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		session := getSession(r)
		fmt.Printf("Client connected: %s\n", session.id)
		return server
	}, nil)

	// Start server in background goroutine
	addr := ":8080"

	go func() {
		fmt.Println("✅ MCP server running at http://localhost" + addr)
		log.Fatal(http.ListenAndServe(addr, handler))
	}()

	// Wait a moment for server to start
	time.Sleep(200 * time.Millisecond)

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Enter X: (or 'exit' to quit) ")
		xStr, _ := reader.ReadString('\n')
		xStr = strings.TrimSpace(xStr)

		if xStr == "exit" {
			break

		}
		x, err := strconv.Atoi(xStr)

		if err != nil {
			fmt.Println("Invalid X:", err)
			continue
		}

		fmt.Print("Enter Y: (or type 'exit' to quit) ")
		yStr, _ := reader.ReadString('\n')
		yStr = strings.TrimSpace(yStr)

		if yStr == "exit" {
			break

		}

		y, err := strconv.Atoi(yStr)
		if err != nil {
			log.Fatalf("invalid Y: %v", err)
			continue
		}

		// Create client and connect to server
		ctx := context.Background()
		client := mcp.NewClient(&mcp.Implementation{
			Name:    "client",
			Version: "v0.0.1",
		}, nil)

		transport := &mcp.SSEClientTransport{
			Endpoint: "http://localhost:8080",
		}

		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			log.Fatalf("failed to connect to server: %v", err)
		}

		// Call the "add" tool
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "add",
			Arguments: map[string]any{"x": x, "y": y},
		})
		if err != nil {
			log.Fatalf("tool call failed: %v", err)
			continue
		}

		fmt.Println("Result:", res.Content[0].(*mcp.TextContent).Text)
	}

}
