package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/akamensky/argparse"
	"github.com/gorilla/websocket"
)

// DebugData is JSON structure returned by Chromium
type DebugData struct {
	Description          string `json:"description"`
	DevtoolsFrontendURL  string `json:"devtoolsFrontendUrl"`
	FaviconURL           string `json:"faviconUrl"`
	ID                   string `json:"id"`
	Title                string `json:"title"`
	PageType             string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// WebsocketResponseRoot is the raw response from Chromium websocket
type WebsocketResponseRoot struct {
	ID     int                     `json:"id"`
	Result WebsocketResponseNested `json:"result"`
}

// WebsocketResponseNested is the object within the raw response from Chromium websocket
type WebsocketResponseNested struct {
	Cookies []Cookie `json:"cookies"`
}

// Cookie is JSON structure returned by Chromium websocket
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Size     int     `json:"size"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	Session  bool    `json:"session"`
	SameSite string  `json:"sameSite"`
	Priority string  `json:"priority"`
}

// LightCookie is a JSON structure for the cookie with only the name, value, domain, path, and (modified) expires fields
type LightCookie struct {
	Name    string  `json:"name"`
	Value   string  `json:"value"`
	Domain  string  `json:"domain"`
	Path    string  `json:"path"`
	Expires float64 `json:"expirationDate"`
}

// GetDebugData interacts with Chromium debug port to obtain the JSON response of open tabs/installed extensions
func GetDebugData(debugPort string) []DebugData {
	var debugURL = "http://localhost:" + debugPort + "/json"
	resp, err := http.Get(debugURL)
	if err != nil {
		log.Fatalf("Failed to get debug data: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}

	var debugList []DebugData
	if err := json.Unmarshal(body, &debugList); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	return debugList
}

// PrintDebugData takes the JSON response from Chromium and prints open tabs and installed extensions
func PrintDebugData(debugList []DebugData, grep string) {
	grepFlag := len(grep) > 0

	for _, value := range debugList {
		if !grepFlag || strings.Contains(value.Title, grep) || strings.Contains(value.URL, grep) {
			fmt.Printf("Title: %s\n", value.Title)
			fmt.Printf("Type: %s\n", value.PageType)
			fmt.Printf("URL: %s\n", value.URL)
			fmt.Printf("WebSocket Debugger URL: %s\n\n", value.WebSocketDebuggerURL)
		}
	}
}

// DumpCookies interacts with the webSocketDebuggerUrl to obtain Chromium cookies
func DumpCookies(debugList []DebugData, format string, grep string) {
	grepFlag := len(grep) > 0
	websocketURL := debugList[0].WebSocketDebuggerURL

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, websocketURL, nil)
	if err != nil {
		log.Fatalf("Failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Set read limit
	conn.SetReadLimit(10 * 1024 * 1024) // 10 MB

	message := `{"id": 1, "method":"Network.getAllCookies"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}

	_, rawResponse, err := conn.ReadMessage()
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	var websocketResponseRoot WebsocketResponseRoot
	if err := json.Unmarshal(rawResponse, &websocketResponseRoot); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if format == "raw" {
		fmt.Printf("%s\n", rawResponse)
		return
	}

	if format == "modified" {
		var lightCookieList []LightCookie

		for _, value := range websocketResponseRoot.Result.Cookies {
			if !grepFlag || strings.Contains(value.Name, grep) || strings.Contains(value.Domain, grep) {
				lightCookie := LightCookie{
					Name:    value.Name,
					Value:   value.Value,
					Domain:  value.Domain,
					Path:    value.Path,
					Expires: float64(time.Now().Unix() + (10 * 365 * 24 * 60 * 60)),
				}

				lightCookieList = append(lightCookieList, lightCookie)
			}
		}

		lightCookieJSON, err := json.Marshal(lightCookieList)
		if err != nil {
			log.Fatalf("Failed to marshal JSON: %v", err)
		}
		fmt.Printf("%s\n", lightCookieJSON)
		return
	}

	for _, value := range websocketResponseRoot.Result.Cookies {
		if !grepFlag || strings.Contains(value.Name, grep) || strings.Contains(value.Domain, grep) {
			fmt.Printf("name: %s\n", value.Name)
			fmt.Printf("value: %s\n", value.Value)
			fmt.Printf("domain: %s\n", value.Domain)
			fmt.Printf("path: %s\n", value.Path)
			fmt.Printf("expires: %f\n", value.Expires)
			fmt.Printf("size: %d\n", value.Size)
			fmt.Printf("httpOnly: %t\n", value.HTTPOnly)
			fmt.Printf("secure: %t\n", value.Secure)
			fmt.Printf("session: %t\n", value.Session)
			fmt.Printf("sameSite: %s\n", value.SameSite)
			fmt.Printf("priority: %s\n\n", value.Priority)
		}
	}
}

func ClearCookies(debugList []DebugData) {
	websocketURL := debugList[0].WebSocketDebuggerURL

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, websocketURL, nil)
	if err != nil {
		log.Fatalf("Failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Set read limit
	conn.SetReadLimit(10 * 1024 * 1024) // 10 MB

	message := `{"id": 1, "method": "Network.clearBrowserCookies"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}
}

func LoadCookies(debugList []DebugData, load string) {
	content, err := os.ReadFile(load)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	websocketURL := debugList[0].WebSocketDebuggerURL

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, websocketURL, nil)
	if err != nil {
		log.Fatalf("Failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Set read limit
	conn.SetReadLimit(10 * 1024 * 1024) // 10 MB

	message := fmt.Sprintf(`{"id": 1, "method":"Network.setCookies", "params":{"cookies":%s}}`, content)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}
}

func main() {
	parser := argparse.NewParser("WhiteChocolateMacademia", "Interact with Chromium-based browsers' debug port to view open tabs, installed extensions, and cookies")

	debugPort := parser.String("p", "port", &argparse.Options{Required: true, Help: "{REQUIRED} - Debug port"})
	dump := parser.String("d", "dump", &argparse.Options{Required: false, Help: "{ pages || cookies } - Dump open tabs/extensions or cookies"})
	format := parser.String("f", "format", &argparse.Options{Required: false, Help: "{ raw || human || modified } - Format when dumping cookies"})
	grep := parser.String("g", "grep", &argparse.Options{Required: false, Help: "Narrow scope of dumping to specific name/domain"})
	load := parser.String("l", "load", &argparse.Options{Required: false, Help: "File name for cookies to load into browser"})
	clear := parser.String("c", "clear", &argparse.Options{Required: false, Help: "Clear cookies before loading new cookies"})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Printf("%s\n", parser.Usage(err))
		os.Exit(1)
	}

	if *dump != "" {
		if *dump == "pages" {
			debugList := GetDebugData(*debugPort)
			PrintDebugData(debugList, *grep)
		}
		if *dump == "cookies" {
			debugList := GetDebugData(*debugPort)
			DumpCookies(debugList, *format, *grep)
		}
	}

	if *clear != "" {
		debugList := GetDebugData(*debugPort)
		ClearCookies(debugList)
	}

	if *load != "" {
		debugList := GetDebugData(*debugPort)
		LoadCookies(debugList, *load)
	}
}
