# ntlm-response

An NTLM site checker for telegraf modeled after the http_response plugin.

## Configuration:
```json
{
"urls": [
        "https://www.google.com"
    ],
    "http_proxy": "",
    "response_timeout": 5000,
    "method": "GET",
    "username": "domain\\user",
    "password": "pass",
    "body": "",
    "response_body_field": "",
    "response_body_max_size": "",
    "response_string_match": "",
    "response_status_code": 0,
    "headers": {
        "Host": "github.com"
    },
    "http_header_tags": {
        "HTTP_HEADER": "TAG_NAME"
    }
}
```

## Metrics:

    http_response
        tags:
            server (target URL)
            method (request method)
            status_code (response status code)
            result (see below)
        fields:
            response_time (float, seconds)
            content_length (int, response body length)
            response_string_match (int, 0 = mismatch / body read error, 1 = match)
            response_status_code_match (int, 0 = mismatch, 1 = match)
            http_response_code (int, response status code)
            result_code (int, see below)

### result / result_code

Upon finishing polling the target server, the plugin registers the result of the operation in the result tag, and adds a numeric field called result_code corresponding with that tag value.

This tag is used to expose network and plugin errors. HTTP errors are considered a successful connection.

|Tag value |	Corresponding field value |	Description |
|---|---|---|
|success |0 |The HTTP request completed, even if the HTTP code represents an error|
|response_string_mismatch |1 |The option response_string_match was used, and the body of the response didn't match the regex. |HTTP errors with content in their body (like 4xx, 5xx) will trigger this error
|body_read_error |2 |The option response_string_match was used, but the plugin wasn't able to read the body of the response. |Responses with empty bodies (like 3xx, HEAD, etc) will trigger this error. Or the option response_body_field was used and the content of the response body was not a valid utf-8. Or the size of the body of the response exceeded the response_body_max_size
|connection_failed |3 |Catch all for any network error not specifically handled by the plugin|
|timeout |4 |The plugin timed out while awaiting the HTTP connection to complete|
|dns_error |5 |There was a DNS error while attempting to connect to the host (not returned)|
|response_status_code_mismatch |6 |The option response_status_code_match was used, and the status code of the response didn't |match the value.