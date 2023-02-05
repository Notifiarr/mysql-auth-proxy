# Notifiarr MySQL Auth Proxy

This auth proxy is used to direct traffic on Notifiarr.com using the Nginx auth proxy module.

### Example Nginx Config

```nginx
log_format local '$host $remote_addr $auth_idnt $auth_user [$time_local] '
    '"$request" $status $body_bytes_sent '
    '"$http_referer" "$http_user_agent" '
    'req=$request_time con="$upstream_connect_time" hed="$upstream_header_time" res="$upstream_response_time"';
    

# Allow http username to override x-api-key header, but only if it's not blank.
map $remote_user $remote_api_key {
  default $remote_user;
  ''      $http_x_api_key;
}

# Extract API Key from URL.
# Optional, because the auth proxy parses the URL too.
map $request_uri $incoming_api_key {
  ~^/api/v[^/]+/[^/]+/[^/]+/([^?\$]+) $1;
  default                     $remote_api_key;
}

map $incoming_api_key $outgoing_api_key {
  ''      $auth_key;
  default $incoming_api_key;
}

# Pick a new Host header based on proxy environment returned.
# This is what we use this auth proxy for.
map $proxy_env $redirect_host {
  default notifiarr.$proxy_env;
  ''      "notifiarr.com";
  'live'  "notifiarr.com";
}

server {
  set $server https://backend.host;
  set $authproxy http://proxy.host:8080;
  server_name notifiarr.com;
  access_log  /config/log/nginx/notifiarr_access.log local;

  listen   443 ssl http2;
  include  /config/nginx/ssl.conf;
  include  /config/nginx/proxy.conf;

  location / {
    proxy_pass $server$request_uri;
  }

  location /api {
    auth_request /auth;
    auth_request_set $proxy_env $upstream_http_X_Environment;
    auth_request_set $auth_user $upstream_http_X_Username;
    auth_request_set $auth_key $upstream_http_X_API_Key;
    auth_request_set $auth_idnt $upstream_http_X_UserID;

    proxy_set_header host $redirect_host;
    proxy_set_header x-api-key $remote_api_key;
    proxy_pass $server$request_uri;
  }

  location = /auth {
    internal;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header X-Original-URI $request_uri;
    proxy_set_header X-API-Key $incoming_api_key;
    proxy_set_header X-Server $http_X_Server;
    proxy_pass $authproxy/auth;
  }
}
```

### Example Docker Compose

```yaml
  auth-proxy:
    image: ghcr.io/notifiarr/mysql-auth-proxy:main
    container_name: auth
    environment:
      - MYSQL_HOST=mysqlhost:3306
      - MYSQL_NAME=mysql_db
      - MYSQL_USER=proxy
      - MYSQL_PASS_FILE=/password
    ports:
      - 8080:8080
    restart: unless-stopped
    volumes:
      - /home/swag/.mysqlsecret:/password:ro
```

## Good Luck!

This app is pretty small and lightweight. It can be cross compiled. It can be easily adapted to other uses of a MySQL auth proxy for Nginx.

If you need a custom auth proxy for Nginx and you know Go, this is a good start. Good luck.
