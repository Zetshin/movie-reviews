#=========================================================================#
# MovieClub TH (Docker) - HTTP: ACME challenge + redirect to HTTPS        #
# Installed by deploy/hestia/install.sh into                              #
#   /usr/local/hestia/data/templates/web/nginx/                           #
#=========================================================================#

server {
	listen      %ip%:%proxy_port%;
	server_name %domain_idn% %alias_idn%;

	# Server-level logs so every request counts toward Hestia bandwidth
	# (v-update-web-domain-traff reads the .bytes file) and web stats.
	access_log  /var/log/%web_system%/domains/%domain%.log combined;
	access_log  /var/log/%web_system%/domains/%domain%.bytes bytes;
	error_log   /var/log/%web_system%/domains/%domain%.error.log error;

	include %home%/%user%/conf/web/%domain%/nginx.conf_*;

	location ^~ /.well-known/acme-challenge/ {
		root %docroot%;
	}

	location / {
		return 301 https://$host$request_uri;
	}
}
