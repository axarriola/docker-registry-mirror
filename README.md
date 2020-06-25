# docker-registry-mirror
Mirror all docker repositories from one registry to another using skopeo.
With this repository you can build a docker image containing skopeo and the registry mirror app written in go. Specify source and destination registries
in config.yml, this application will get a list of all repositories from the source registry and use `skopeo sync`command to mirror them to the 
destination registry. This process will be executed every $INTERVAL seconds.

# Requirements
- docker
- docker-compose

# How to use
Start by building the docker image running
```
$ ./build.sh
```

Place your certificate, certificate key and rootchain in the host machine under /var/certs (you can change the location in docker-compose.yml).
If you won't use certificates you can delete that volume mount, or leave it.

Copy the example config:
```
$ mkdir -p /var/registry-mirror
$ cp config.yml.example /var/registry-mirror/config.yml
```
Set your values:
 * **host:** the host:port and path for the registry
 * **user:** if credentials are used, otherwise leave empty
 * **pass:** if credentials are used, otherwise leave empty
 * **ssl:** true to use https, false for http
 * **api:** in case the api path for the registry differs, add it here

Create directory for registry volume:
```
$ mkdir /var/registry
```

Run docker-compose up:
```
$ docker-compose up -d
```
The file docker-compose.yml includes a registry, if you only want the mirror:
```
docker-compose up -d registry-mirror
```

Other values can be changed in the docker-compose.yml variables, like INTERVAL and config path (CONFIG) and the certificate path/names.
