import { NtlmClient, NtlmCredentials } from 'axios-ntlm';
import { AxiosInstance, AxiosRequestConfig, AxiosResponse } from 'axios';
import { readFileSync } from 'fs';

interface config {
    urls: Array<string>;
    http_proxy?: string;
    response_timeout?: number;
    method?: "GET" | "get" | "delete" | "DELETE" | "head" | "HEAD" | "options" | "OPTIONS" | "post" | "POST" | "put" | "PUT" | "patch" | "PATCH" | "purge" | "PURGE" | "link" | "LINK" | "unlink" | "UNLINK";
    username: string;
    password: string;
    body?: string;
    response_body_field?: string;
    response_body_max_size?: string;
    response_string_match?: string;
    response_status_code?: number;
    headers?: { [header: string]: string };
    http_header_tags?: { [header: string]: string };
}

if (process.argv.length != 3) {
    console.log(
        `NTLM-Response
Usage: ./ntlm-response <config file>

Returns an InfluxDB line protocol formatted set of metrics for targets defined in the config file

Sample Config:

{
    "urls": [
        "https://www.google.com"
    ],
    "http_proxy": "",
    "response_timeout": 5000,
    "method": "get",
    "username": "domain\\username",
    "password": "password",
    "body": "",
    "response_body_field": "",
    "response_body_max_size": "",
    "response_string_match": "",
    "response_status_code": 0,
    "headers": {
        "Host": "githubcom"
    },
    "http_header_tags": {
        "HTTP_HEADER": "TAG_NAME"
    }
}

Source: https://github.com/Catbuttes/ntlm-response
`
    );

process.exit(1);
}

let configFile = process.argv[2].toString();
let configString = readFileSync(configFile);
let config: config = JSON.parse(configString.toString());

(async () => {

    let splitCreds = config.username.split("\\");
    let creds: NtlmCredentials = {
        domain: splitCreds[0],
        username: splitCreds[1],
        password: config.password
    };

    let client = NtlmClient(creds);

    try {
        let pending: Array<Promise<any>> = new Array<Promise<any>>();
        config.urls.forEach(async url => {
            let request: AxiosRequestConfig = {
                url: url,
                method: config.method ?? "GET",
                headers: config.headers,
                data: config.body,
                timeout: config.response_timeout ?? 0
            }

            pending.push(getRequest(client, request));

        });

        await Promise.all(pending)


    }
    catch (err) {
        console.log("err" + JSON.stringify(err))
    }


})()

async function getRequest(client: AxiosInstance, request: AxiosRequestConfig) {

    try {
        let start = Date.now();
        let response = await client(request);
        let end = Date.now();

        let duration = (end - start) / 1000;

        let metric: string =
            "ntlm_response"
            + ",method=" + request.method!.toLowerCase()
            + ",server=" + request.url
        if (response !== undefined) {
            metric = metric + ",status_code=" + response.status
        }
        else {
            metric = metric + ",status_code=408"
        }
        metric = metric + ",result=" + getResult(response)

        if (response !== undefined && config.http_header_tags !== undefined) {
            for (let header in config.http_header_tags) {
                if (Object.prototype.hasOwnProperty.call(config.http_header_tags, header)) {
                    let tag = config.http_header_tags[header];
                    if (response.headers[header] !== undefined) {
                        metric = metric + "," + tag + "=" + response.headers[header].toString();
                    }
                }
            }
        }

        metric = metric + " "
        if (response !== undefined) {
            metric = metric + ",content_length=" + response.data.length + "i"
        }
        else {
            metric = metric + ",content_length=0"
        }

        if (response !== undefined) {
            metric = metric + ",http_response_code=" + response.status + "i"
        }
        else {
            metric = metric + ",http_response_code=408i"
        }
        metric = metric + ",response_time=" + duration
            + ",response_status_code_match=" + matchResponseCode(response)
            + ",response_string_match=" + matchResponseString(response)
            + " "
            + Date.now()

        console.log(metric);

    }
    catch (ex) {
        console.log("Err" + ex)
    }


}

function getResult(response: AxiosResponse): string {

    if (response == undefined) {
        return "3";
    }

    if (response.statusText == "ECONNABORTED") {
        return "4";
    }

    if (response.data == undefined) {
        return "2";
    }

    if (matchResponseCode(response) !== "1") {
        return "6";
    }

    if (matchResponseString(response) !== "1") {
        return "1";
    }

    return "0";
}

function matchResponseString(response: AxiosResponse): string {
    if (config.response_string_match === undefined) {
        return "1"
    }

    if (config.response_string_match === "") {
        return "1"
    }

    if (response === undefined) {
        return "0"
    }

    if (response.data === undefined) {
        return "0"
    }

    if (response.data.length === 0) {
        return "0"
    }

    let regex = new RegExp(config.response_string_match!)

    if (regex.test(response.data)) {
        return "1"
    }

    return "0";
}

function matchResponseCode(response: AxiosResponse): string {

    if (config.response_status_code == undefined) {
        return "1";
    }
    if (config.response_status_code === 0) {
        return "1";
    }
    if (response === undefined) {
        if (config.response_status_code === 408) {
            return "1";
        }

        return "0";
    }

    if (response.status === config.response_status_code) {
        return "1";
    }
    return "0";
}

