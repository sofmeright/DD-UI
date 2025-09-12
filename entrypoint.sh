#!/bin/bash
set -e

# Generate config.js file with current environment variables
echo "// Auto-generated configuration file" > /app/ui/dist/config.js
echo "window.DDUI_CONFIG = {" >> /app/ui/dist/config.js
echo "  LOG_LEVEL: '${DDUI_LOG_LEVEL:-info}'," >> /app/ui/dist/config.js
echo "  VERSION: '$(date +%Y%m%d-%H%M%S)'," >> /app/ui/dist/config.js
echo "  GENERATED_AT: '$(date -Iseconds)'" >> /app/ui/dist/config.js
echo "};" >> /app/ui/dist/config.js

echo "Generated config.js with LOG_LEVEL=${DDUI_LOG_LEVEL:-info}"

# Execute the original command
exec "$@"
