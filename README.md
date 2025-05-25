# Transparent OpenAI API Proxy

A lightweight, transparent proxy server for the OpenAI API that logs and forwards requests and responses. This proxy is designed as a debugging tool to inspect API traffic between clients and the OpenAI API.

## Features

- Proxies all requests to the OpenAI API
- Detailed logging of requests and responses
- Support for streaming responses (SSE)
- Configurable via command-line flags or environment variables
- Minimal dependencies (only uses Go standard library and godotenv)

## Installation

### Prerequisites

- Go 1.18 or higher

### Setup

1. Clone the repository:

```bash
git clone https://github.com/gzuuus/transparent-oai-api.git
cd transparent-oai-api
```

2. Install dependencies:

```bash
go mod tidy
```

3. Copy the example environment file and configure it:

```bash
cp .env.example .env
```

4. Edit the `.env` file with your OpenAI API key and other settings.

## Configuration

The proxy can be configured using command-line flags or environment variables. Command-line flags take precedence over environment variables.

### Command-line Flags

```
  -port, -p string
        Port for the proxy server to listen on
  -url, -u string
        Base URL for the OpenAI API
  -key, -k string
        Your OpenAI API key
  -req, -r
        Enable request logging (default true)
  -resp, -s
        Enable response logging (default true)
  -stdout, -o
        Log to standard output (default true)
  -file, -f string
        File to log requests and responses
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|----------|
| `OPENAI_BASE_URL` | Base URL for the OpenAI API | `https://api.openai.com/v1` |
| `OPENAI_API_KEY` | Your OpenAI API key | - |
| `PORT` | Port for the proxy server to listen on | `8080` |
| `LOG_REQUESTS` | Enable request logging | `true` |
| `LOG_RESPONSES` | Enable response logging | `true` |
| `LOG_TO_STDOUT` | Log to standard output | `true` |
| `REQUEST_LOG_FILE` | File to log requests and responses | - |

## Usage

1. Start the proxy server with default settings:

```bash
go run main.go
```

Or with custom configuration via command-line flags (using the shorter options):

```bash
go run main.go -p 9000 -k your_api_key -f api_logs.txt
```

2. Configure your OpenAI client to use the proxy by setting the base URL to `http://localhost:8080` (or whatever port you configured).

3. Make API requests as usual. The proxy will forward them to the OpenAI API and log the details.

## How It Works

1. The proxy server receives API requests from clients
2. It logs the request details (headers, body, etc.)
3. It forwards the request to the actual OpenAI API
4. It receives the response from the OpenAI API
5. It logs the response details
6. It forwards the response back to the client

This allows you to see exactly what data is being sent to and received from the OpenAI API, which is useful for debugging and development.

## License

MIT
