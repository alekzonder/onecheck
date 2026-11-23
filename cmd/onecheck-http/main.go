// Command onecheck-http is the onecheck module that makes HTTP/HTTPS requests.
package main

import (
	"encoding/json"

	"github.com/alekzonder/onecheck/internal/version"
	"github.com/alekzonder/onecheck/pkg/module"
)

const paramsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["url"],
  "properties": {
    "url": {"type": "string", "pattern": "^https?://", "description": "http:// or https:// URL"},
    "method": {"type": "string", "description": "HTTP method, default GET"},
    "headers": {"type": "object", "additionalProperties": {"type": "string"}, "description": "request headers"},
    "body": {"type": "string", "description": "request body"},
    "follow_redirects": {"type": "boolean", "description": "follow redirects, default true"},
    "insecure_skip_verify": {"type": "boolean", "description": "do not verify the TLS certificate, default false"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "http",
		Version:      version.Version,
		Description:  "Make an HTTP/HTTPS request and return status, headers, body, timing and certificate lifetime",
		ParamsSchema: json.RawMessage(paramsSchema),
		Fields: map[string]module.FieldInfo{
			"status":         {Type: module.TypeNumber, Description: "HTTP status code"},
			"headers":        {Type: module.TypeObject, Description: "response headers; names in lower case, repeated values joined with \", \"", Output: true},
			"body":           {Type: module.TypeString, Description: "response body", Output: true},
			"duration":       {Type: module.TypeDuration, Description: "seconds from sending the request to reading the whole body"},
			"tls_expires_in": {Type: module.TypeDuration, Description: "seconds until the server certificate expires (https only)"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
