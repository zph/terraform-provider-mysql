#!/usr/bin/env bash

# TiDB Playground wrapper script for testing
# Uses TiUP playground for faster TiDB cluster startup

set -e

VERSION=${MYSQL_VERSION:-7.5.2}
PORT=${MYSQL_PORT:-4000}
MODE=${1:-start}  # start or stop

# Ensure TiUP is in PATH
export PATH=$HOME/.tiup/bin:$PATH

if ! command -v tiup &> /dev/null; then
    echo "TiUP not found. Installing..."
    curl --proto '=https' --tlsv1.2 -sSf https://tiup-mirrors.pingcap.com/install.sh | sh
    export PATH=$HOME/.tiup/bin:$PATH
fi

if [ "$MODE" = "start" ]; then
    echo "==> Starting TiDB Playground v${VERSION} on port ${PORT}..."
    
    # Clean up any existing playground instances
    pkill -f "tiup playground" || true
    sleep 1
    
    # Start playground in background
    tiup playground ${VERSION} \
        --db 1 \
        --kv 1 \
        --pd 1 \
        --tiflash 0 \
        --without-monitor \
        --host 0.0.0.0 \
        --db.port ${PORT} \
        > /tmp/tidb-playground-${PORT}.log 2>&1 &
    
    PLAYGROUND_PID=$!
    echo $PLAYGROUND_PID > /tmp/tidb-playground-${PORT}.pid
    
    # Wait for TiDB to be ready (max 120 seconds)
    echo "Waiting for TiDB to be ready..."
    for i in {1..120}; do
        if mysql -h 127.0.0.1 -P ${PORT} -u root -e 'SELECT 1' >/dev/null 2>&1; then
            echo "TiDB is ready!"
            exit 0
        fi
        sleep 1
        if [ $((i % 10)) -eq 0 ]; then
            printf "."
        fi
    done
    
    echo ""
    echo "ERROR: TiDB failed to start within 120 seconds"
    echo "Last 20 lines of playground log:"
    tail -20 /tmp/tidb-playground-${PORT}.log || true
    exit 1
    
elif [ "$MODE" = "stop" ]; then
    echo "==> Stopping TiDB Playground..."
    
    # Kill by PID if available
    if [ -f /tmp/tidb-playground-${PORT}.pid ]; then
        PID=$(cat /tmp/tidb-playground-${PORT}.pid)
        kill $PID 2>/dev/null || true
        rm /tmp/tidb-playground-${PORT}.pid
    fi
    
    # Kill any remaining tiup playground processes
    pkill -f "tiup playground" || true
    
    # Clean up log file
    rm -f /tmp/tidb-playground-${PORT}.log
    
    echo "TiDB Playground stopped"
fi
