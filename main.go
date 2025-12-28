package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// text/event-stream, just so the output below doesn't use random event ids
type AddParams struct {
	X int `json:"x"`
	Y int `json:"y"`
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

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "server", Version: "v0.1.0", WebsiteURL: "http://localhost:8080"}, nil)

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

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {

		defer r.Body.Close()

		scanner := bufio.NewScanner(r.Body)

		for scanner.Scan() {
			fmt.Println(len(scanner.Bytes()) == 6)
		}
		return server
	},
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)

	go func() {
		port := ":8080"
		fmt.Println("✅ MCP server running at http://localhost" + port)
		log.Fatal(http.ListenAndServe(port, handler))
	}()
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

		client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil)

		ctx := context.Background()

		t1, t2 := mcp.NewInMemoryTransports()
		if _, err := server.Connect(ctx, t1, nil); err != nil {
			log.Fatal(err)
		}
		session, err := client.Connect(ctx, t2, nil)
		if err != nil {
			log.Fatal(err)
		}

		defer session.Close()

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
		resp := mustPostMessage(`{"jsonrpc": "2.0", "id": 1, "method":"initialize", "params": {}}`, "http://localhost:8080")
		fmt.Println(resp)
	}

}

func mustPostMessage(msg, url string) string {

	msg = "{\"apple\": \"green\"}" // Example JSON payload as a string
	// strings.NewReader returns an io.Reader that reads from the string 'msg'
	bodyReader := strings.NewReader(msg)

	req := orFatal(http.NewRequest("POST", "http://localhost:8080", bodyReader))
	req.Header["Content-Type"] = []string{"application/json"}
	req.Header["Accept"] = []string{"application/json", "text/event-stream"}

	resp := orFatal(http.DefaultClient.Do(req))
	defer resp.Body.Close()
	body := orFatal(io.ReadAll(resp.Body))
	return string(body)

}

func orFatal[T any](t T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return t
}
