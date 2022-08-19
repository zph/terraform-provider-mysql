#!/usr/bin/env bash
# This file creates minimal TiDB cluster for tests

REALPATH=$(which realpath)
if [ -z $REALPATH ]; then
  realpath() {
    [[ $1 == /* ]] && echo "$1" || echo "$PWD/${1#./}"
  }
fi

# Set up constants
SCRIPT_PATH=$(realpath $(dirname "$0"))
TEMPDIR=$(mktemp -d)
DOCKER=$(which docker)
TAG_VERSION="v6.1.0"
SCRIPT_INIT=false
MYIP="127.0.0.1"
RUNNING_CONTAINERS=""
export MYSQL_PORT=${MYSQL_PORT:-4000}

# Sanity checks
if [ -z "$DOCKER" ]; then
  echo "Missing docker binary"
  exit 2
fi

# A better class of script...
set -o errexit  # Exit on most errors (see the manual)
set -o errtrace # Make sure any error trap is inherited
set -o nounset  # Disallow expansion of unset variables
set -o pipefail # Use last non-zero exit code in a pipeline

function script_usage() {
  cat <<EOF
Usage:
     --mysql-port	<MYSQL_PORT>		TiDB Listen port
     --init   										Init TiDB cluster
     --destroy										Destroy resources
     -h|--help  									Displays this help
EOF
}

function parse_params() {
  local param
  while [[ $# -gt 0 ]]; do
    param="$1"
    case $param in
    --port)
			if [ -z "$2" ]; then
				echo missing param
			fi
      export MYSQL_PORT=${2:-4000}
			shift # past argument
			shift # past value
      ;;
    --init)
      export SCRIPT_INIT=true
			shift
      ;;
    --destroy)
			if [ -z "$SCRIPT_INIT" ]; then
				echo "Cant destroy and init at once"
				exit 1
			fi
			destroy_cluster
			exit 0
      ;;
    -h | --help)
      script_usage
			shift
      exit 0
      ;;
    *)
      echo "Invalid parameter was provided: $param"
			script_usage
			shift
      exit 1
      ;;
    esac
  done
}



# My IP detection source code
function get_my_ip() {
	if [ ! -d "$SCRIPT_PATH/../bin" ]; then
		mkdir -p $SCRIPT_PATH/../bin
	fi
cat <<\__END__ > $SCRIPT_PATH/../bin/myip.go
package main

import (
	"errors"
	"fmt"
	"net"
	"os"
)

func main() {

	ip, err := externalIP()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(ip)

}

func externalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("are you connected to the network?")
}
__END__
gofmt -w $SCRIPT_PATH/../bin/myip.go || return 1
MYIP=$(go run $SCRIPT_PATH/../bin/myip.go)
}

function destroy_cluster() {
	RUNNING_CONTAINERS=$(${DOCKER} ps -a -q -f name=tidb -f name=tikv -f name=pd)
	echo "==> Removing any existing TiDB cluster components"
	if [ ! -z "$RUNNING_CONTAINERS" ]; then
		${DOCKER} rm -f $RUNNING_CONTAINERS >/dev/null 2>&1
	fi
}

function show_docker_logs_and_exit() {
	docker ps -a -q -f name=$1 |xargs docker logs --details 2>&1
	echo "Error with $1 component. For debugging use:"
	echo "docker ps -a -q -f name=$1 |xargs docker logs"
	exit 1
}

function run_pd() {
	local _myip=$1
	echo "==> Pulling up PD component"
	${DOCKER} run -d --name pd1 \
		-p 2379:2379 \
		-p 2380:2380 \
		-v /etc/localtime:/etc/localtime:ro \
		-v /data:/data \
		-h pd1 \
		pingcap/pd:$TAG_VERSION \
		--name="pd1" \
		--data-dir="$TEMPDIR/pd1" \
		--client-urls="http://0.0.0.0:2379" \
		--advertise-client-urls="http://$_myip:2379" \
		--peer-urls="http://0.0.0.0:2380" \
		--advertise-peer-urls="http://$_myip:2380" \
		--initial-cluster="pd1=http://$_myip:2380" >/dev/null 2>&1 || show_docker_logs_and_exit pd
}

function run_tikv() {
	local _myip=$1
	echo "==> Pulling up TiKV component"
	${DOCKER} run -d --name tikv1 \
		-p 20160:20160 \
		-p 20180:20180 \
		-v /etc/localtime:/etc/localtime:ro \
		-v /data:/data \
		-h tikv1 \
		pingcap/tikv:v4.0.0 \
		--addr="0.0.0.0:20160" \
		--advertise-addr="$_myip:20160" \
		--status-addr="0.0.0.0:20180" \
		--data-dir="$TEMPDIR/tikv1" \
		--pd="$_myip:2379" >/dev/null 2>&1 || show_docker_logs_and_exit tikv
}

function run_tidb() {
	local _myip=$1
	local _mysql_port=$2
	echo "==> Pulling up TiDB component"
	${DOCKER} run -d --name tidb \
		-p $_mysql_port:$_mysql_port \
		-p 10080:10080 \
		-v /etc/localtime:/etc/localtime:ro \
		-h tidb \
		pingcap/tidb:$TAG_VERSION \
		--store=tikv \
		-P $_mysql_port \
		--path="$_myip:2379" >/dev/null 2>&1 || show_docker_logs_and_exit tidb
}

function main() {
	local _myip
	  parse_params "$@"
		if [[ "$SCRIPT_INIT" = "true" ]]; then
			echo "==> Pulling up TiDB cluster with TiKV and TB components"
		  get_my_ip && \
			destroy_cluster && \
			run_pd $MYIP && \
			run_tikv $MYIP && \
			run_tidb $MYIP $MYSQL_PORT

		else
			script_usage
		fi
}

main "$@"
