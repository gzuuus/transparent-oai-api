package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	OpenAIBaseURL  string
	OpenAIAPIKey   string
	LogRequests    bool
	LogResponses   bool
	LogToStdout    bool
	RequestLogFile string
}

type RequestLogger struct {
	LogFile     *os.File
	LogToFile   bool
	LogToStdout bool
}

func NewRequestLogger(logFile string, logToStdout bool) (*RequestLogger, error) {
	logger := &RequestLogger{
		LogToStdout: logToStdout,
	}

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		logger.LogFile = f
		logger.LogToFile = true
	}

	return logger, nil
}

func (l *RequestLogger) Close() {
	if l.LogFile != nil {
		l.LogFile.Close()
	}
}

func (l *RequestLogger) LogRequest(r *http.Request, body []byte) {
	timestamp := time.Now().Format(time.RFC3339)
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "==== REQUEST [%s] %s ====\n", reqID, timestamp)
	fmt.Fprintf(&buf, "%s %s %s\n", r.Method, r.URL.Path, r.Proto)

	// Log headers
	fmt.Fprintln(&buf, "Headers:")
	for name, values := range r.Header {
		// Skip Authorization header content for security
		if strings.ToLower(name) == "authorization" {
			fmt.Fprintf(&buf, "  %s: Bearer [REDACTED]\n", name)
			continue
		}
		for _, value := range values {
			fmt.Fprintf(&buf, "  %s: %s\n", name, value)
		}
	}

	// Log body if present
	if len(body) > 0 {
		fmt.Fprintln(&buf, "Body:")
		fmt.Fprintln(&buf, string(body))
	}

	logData := buf.String()

	// Write to file if configured
	if l.LogToFile && l.LogFile != nil {
		fmt.Fprintln(l.LogFile, logData)
	}

	// Write to stdout if configured
	if l.LogToStdout {
		fmt.Print(logData)
	}
}

func (l *RequestLogger) LogResponse(reqID string, resp *http.Response, body []byte) {
	timestamp := time.Now().Format(time.RFC3339)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "==== RESPONSE [%s] %s ====\n", reqID, timestamp)
	fmt.Fprintf(&buf, "%s %s\n", resp.Proto, resp.Status)

	// Log headers
	fmt.Fprintln(&buf, "Headers:")
	for name, values := range resp.Header {
		for _, value := range values {
			fmt.Fprintf(&buf, "  %s: %s\n", name, value)
		}
	}

	// Log body if present and not too large
	if len(body) > 0 {
		// Limit body size for logging to prevent huge logs
		maxBodySize := 10000 // 10KB
		bodyToLog := body
		if len(body) > maxBodySize {
			bodyToLog = body[:maxBodySize]
			fmt.Fprintf(&buf, "Body (truncated to %d bytes):\n", maxBodySize)
		} else {
			fmt.Fprintln(&buf, "Body:")
		}
		fmt.Fprintln(&buf, string(bodyToLog))

		if len(body) > maxBodySize {
			fmt.Fprintf(&buf, "... [%d more bytes]\n", len(body)-maxBodySize)
		}
	}

	logData := buf.String()

	// Write to file if configured
	if l.LogToFile && l.LogFile != nil {
		fmt.Fprintln(l.LogFile, logData)
	}

	// Write to stdout if configured
	if l.LogToStdout {
		fmt.Print(logData)
	}
}

type ProxyServer struct {
	Config Config
	Logger *RequestLogger
}

func NewProxyServer(config Config) (*ProxyServer, error) {
	logger, err := NewRequestLogger(config.RequestLogFile, config.LogToStdout)
	if err != nil {
		return nil, err
	}

	return &ProxyServer{
		Config: config,
		Logger: logger,
	}, nil
}

func (s *ProxyServer) Close() {
	if s.Logger != nil {
		s.Logger.Close()
	}
}

func (s *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Generate a request ID if not present
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = fmt.Sprintf("req-%d", time.Now().UnixNano())
		r.Header.Set("X-Request-ID", reqID)
	}

	// Read the request body
	var bodyBytes []byte
	var err error

	if r.Body != nil {
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Log the request if enabled
	if s.Config.LogRequests {
		s.Logger.LogRequest(r, bodyBytes)
	}

	// Create a new request to forward to the OpenAI API
	targetURL := s.Config.OpenAIBaseURL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		http.Error(w, "Error creating proxy request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy headers from original request
	for name, values := range r.Header {
		if strings.ToLower(name) == "host" {
			continue
		}
		for _, value := range values {
			proxyReq.Header.Add(name, value)
		}
	}

	// Set API key if not provided in the request
	if proxyReq.Header.Get("Authorization") == "" && s.Config.OpenAIAPIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+s.Config.OpenAIAPIKey)
	}

	// Create HTTP client with appropriate timeouts
	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	// Make the request to the OpenAI API
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Error forwarding request to OpenAI API: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Set response status code
	w.WriteHeader(resp.StatusCode)

	// Handle streaming responses differently
	isStreaming := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

	if isStreaming {
		if s.Config.LogResponses {
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming not supported", http.StatusInternalServerError)
				return
			}

			buffer := make([]byte, 4096)
			for {
				n, err := resp.Body.Read(buffer)
				if n > 0 {
					chunk := buffer[:n]
					if _, writeErr := w.Write(chunk); writeErr != nil {
						log.Printf("Error writing response chunk: %v", writeErr)
						break
					}
					flusher.Flush()
					s.Logger.LogResponse(reqID, resp, chunk)
				}

				if err != nil {
					if err != io.EOF {
						log.Printf("Error reading response body: %v", err)
					}
					break
				}
			}
		} else {
			io.Copy(w, resp.Body)
		}
	} else {
		// For non-streaming responses
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading response body: %v", err)
			http.Error(w, "Error reading response from OpenAI API", http.StatusInternalServerError)
			return
		}

		if s.Config.LogResponses {
			s.Logger.LogResponse(reqID, resp, responseBody)
		}

		w.Write(responseBody)
	}
}

func loadConfig() Config {
	var config Config

	var flagLogRequests, flagLogResponses, flagLogToStdout bool
	var flagsSet bool

	flag.StringVar(&config.Port, "port", "", "Port for the proxy server to listen on")
	flag.StringVar(&config.Port, "p", "", "Port for the proxy server to listen on (shorthand)")
	
	flag.StringVar(&config.OpenAIBaseURL, "url", "", "Base URL for the OpenAI API")
	flag.StringVar(&config.OpenAIBaseURL, "u", "", "Base URL for the OpenAI API (shorthand)")
	
	flag.StringVar(&config.OpenAIAPIKey, "key", "", "Your OpenAI API key")
	flag.StringVar(&config.OpenAIAPIKey, "k", "", "Your OpenAI API key (shorthand)")
	
	flag.BoolVar(&flagLogRequests, "req", true, "Enable request logging")
	flag.BoolVar(&flagLogRequests, "r", true, "Enable request logging (shorthand)")
	
	flag.BoolVar(&flagLogResponses, "resp", true, "Enable response logging")
	flag.BoolVar(&flagLogResponses, "s", true, "Enable response logging (shorthand)")
	
	flag.BoolVar(&flagLogToStdout, "stdout", true, "Log to standard output")
	flag.BoolVar(&flagLogToStdout, "o", true, "Log to standard output (shorthand)")
	
	flag.StringVar(&config.RequestLogFile, "file", "", "File to log requests and responses")
	flag.StringVar(&config.RequestLogFile, "f", "", "File to log requests and responses (shorthand)")

	flag.Visit(func(f *flag.Flag) {
		flagsSet = true
	})

	flag.Parse()

	_ = godotenv.Load()

	parseBool := func(envVar string, defaultVal bool) bool {
		val := os.Getenv(envVar)
		if val == "" {
			return defaultVal
		}
		boolVal, err := strconv.ParseBool(val)
		if err != nil {
			log.Printf("Warning: Invalid value for %s, using default: %v", envVar, defaultVal)
			return defaultVal
		}
		return boolVal
	}

	if envPort := os.Getenv("PORT"); envPort != "" && config.Port == "" {
		config.Port = envPort
	}
	
	if envURL := os.Getenv("OPENAI_BASE_URL"); envURL != "" && config.OpenAIBaseURL == "" {
		config.OpenAIBaseURL = envURL
	}
	
	if envKey := os.Getenv("OPENAI_API_KEY"); envKey != "" && config.OpenAIAPIKey == "" {
		config.OpenAIAPIKey = envKey
	}

	config.LogRequests = flagLogRequests
	config.LogResponses = flagLogResponses
	config.LogToStdout = flagLogToStdout
	
	if !flagsSet {
		config.LogRequests = parseBool("LOG_REQUESTS", config.LogRequests)
		config.LogResponses = parseBool("LOG_RESPONSES", config.LogResponses)
		config.LogToStdout = parseBool("LOG_TO_STDOUT", config.LogToStdout)
	}
	
	if envLogFile := os.Getenv("REQUEST_LOG_FILE"); envLogFile != "" && config.RequestLogFile == "" {
		config.RequestLogFile = envLogFile
	}

	if config.Port == "" {
		config.Port = "8080"
	}

	if config.OpenAIBaseURL == "" {
		config.OpenAIBaseURL = "https://api.openai.com/v1"
	} else {
		config.OpenAIBaseURL = strings.TrimSuffix(config.OpenAIBaseURL, "/")
	}

	return config
}

func main() {
	config := loadConfig()

	server, err := NewProxyServer(config)
	if err != nil {
		log.Fatalf("Failed to create proxy server: %v", err)
	}
	defer server.Close()

	httpServer := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      server,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Starting OpenAI API proxy server on port %s", config.Port)
	log.Printf("Forwarding requests to %s", config.OpenAIBaseURL)
	log.Printf("Logging: requests=%v, responses=%v, to_stdout=%v, log_file=%s",
		config.LogRequests, config.LogResponses, config.LogToStdout,
		config.RequestLogFile)

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
