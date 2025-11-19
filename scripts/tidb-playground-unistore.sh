#!/usr/bin/env bash

# TiDB Playground wrapper script for fast Unistore-based testing
# Uses TiUP playground with Unistore backend for quick smoke tests

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
else
    # Update TiUP and playground component to ensure latest version
    echo "Updating TiUP and playground component..."
    tiup update --self || true
    tiup update playground || true
fi

# Create temporary TiDB config file for Unistore
TIDB_CONFIG=$(mktemp)
cat > "${TIDB_CONFIG}" <<EOF
[store]
store = "unistore"
EOF

if [ "$MODE" = "start" ]; then
    echo "==> Starting TiDB Playground v${VERSION} with Unistore backend on port ${PORT}..."
    
    # Clean up any existing playground instances
    pkill -f "tiup playground" || true
    sleep 1
    
    # Start playground with Unistore backend
    # Note: Still need PD for metadata, but no TiKV needed
    tiup playground ${VERSION} \
        --db 1 \
        --kv 0 \
        --pd 1 \
        --tiflash 0 \
        --without-monitor \
        --host 0.0.0.0 \
        --db.port ${PORT} \
        --db.config "${TIDB_CONFIG}" \
        > /tmp/tidb-playground-unistore-${PORT}.log 2>&1 &
    
    PLAYGROUND_PID=$!
    echo $PLAYGROUND_PID > /tmp/tidb-playground-unistore-${PORT}.pid
    
    # Unistore starts faster, use shorter timeout (60 seconds)
    TIMEOUT=60
    echo "Waiting for TiDB with Unistore to be ready (max ${TIMEOUT} seconds)..."
    for i in $(seq 1 ${TIMEOUT}); do
        if mysql -h 127.0.0.1 -P ${PORT} -u root -e 'SELECT 1' >/dev/null 2>&1; then
            echo "TiDB with Unistore is ready!"
            rm -f "${TIDB_CONFIG}"
            exit 0
        fi
        sleep 1
        if [ $((i % 5)) -eq 0 ]; then
            printf "."
        fi
    done
    
    echo ""
    echo "ERROR: TiDB with Unistore failed to start within ${TIMEOUT} seconds"
    echo "Last 20 lines of playground log:"
    tail -20 /tmp/tidb-playground-unistore-${PORT}.log || true
    rm -f "${TIDB_CONFIG}"
    exit 1
    
elif [ "$MODE" = "stop" ]; then
    echo "==> Stopping TiDB Playground (Unistore)..."
    
    # Kill by PID if available
    if [ -f /tmp/tidb-playground-unistore-${PORT}.pid ]; then
        PID=$(cat /tmp/tidb-playground-unistore-${PORT}.pid)
        kill $PID 2>/dev/null || true
        rm /tmp/tidb-playground-unistore-${PORT}.pid
    fi
    
    # Kill any remaining tiup playground processes
    pkill -f "tiup playground" || true
    
    # Clean up log file and config
    rm -f /tmp/tidb-playground-unistore-${PORT}.log
    rm -f "${TIDB_CONFIG}" 2>/dev/null || true
    
    echo "TiDB Playground (Unistore) stopped"
fi
