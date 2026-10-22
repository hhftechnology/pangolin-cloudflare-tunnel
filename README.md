[![Discuss on the HHF forum](https://img.shields.io/badge/discuss-forum.hhf.technology-E65100?logo=discourse&logoColor=white)](https://forum.hhf.technology/c/pangolin-newt-tools/pangolin-cloudflare-tunnel)

<div align="center">
    <h1>Pangolin-Cloudflare-Tunnel</h1>
    <p>A bridge between Traefik and Cloudflare Zero-Trust tunnels that enables Pangolin users to leverage Cloudflare's global network.</p>

[![Docker](https://img.shields.io/docker/pulls/hhftechnology/pangolin-cloudflare-tunnel?style=flat-square)](https://hub.docker.com/r/hhftechnology/pangolin-cloudflare-tunnel)
![Stars](https://img.shields.io/github/stars/hhftechnology/pangolin-cloudflare-tunnel?style=flat-square)
[![Discord](https://img.shields.io/discord/994247717368909884?logo=discord&style=flat-square)](https://discord.gg/HDCt9MjyMJ)
</div>

## Overview

This tool synchronizes Traefik routes with Cloudflare Zero-Trust tunnels, providing an alternative or complementary tunneling option for Pangolin deployments. This integration allows you to:

- Expose Pangolin-managed services through Cloudflare's global network
- Take advantage of Cloudflare's DDoS protection and caching capabilities
- Provide an alternative remote access method alongside Pangolin's WireGuard tunnels
- **NEW**: Manage multiple domains across different Cloudflare zones
- **NEW**: Exclude specific resources from Cloudflare tunneling
- **NEW**: Automatic cleanup of DNS records for deleted resources

## Features

- **Multi-Domain/Multi-Zone Support**: Configure multiple domains across different Cloudflare zones
- **Resource Exclusion**: Ignore specific domains or patterns (e.g., Jellyfin for TOS compliance)
- **Automatic DNS Cleanup**: Automatically remove DNS records when resources are deleted
- **TLS Route Filtering**: Optionally skip or include TLS-enabled routes
- **Multiple Entrypoints**: Support for multiple Traefik entrypoints
- **Automatic Synchronization**: Real-time sync between Traefik and Cloudflare
- **Robust Error Handling**: Retry logic with exponential backoff
- **Structured Logging**: Comprehensive logging with configurable verbosity

## Integration with Pangolin

When used with Pangolin:

1. Pangolin manages your internal resources
2. Traefik (used by Pangolin) handles the local routing
3. This tool synchronizes Traefik routes to Cloudflare tunnels
4. Cloudflare provides an additional layer of protection and global distribution

This creates a combination where you can use Pangolin for secure local deployment via Cloudflare tunnels for public-facing services for Unraid/NAS users without opening ports or buying a VPS.

## Configuration

### Required Environment Variables

| Environment Variable     | Type   | Description                                                  |
| :----------------------- | ------ | ------------------------------------------------------------ |
| CLOUDFLARED_TOKEN        | String | Token for the `cloudflared` daemon. This is the token provided after [creating a tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/tunnel-guide/#1-create-a-tunnel). |
| CLOUDFLARE_API_TOKEN     | String | A valid [Cloudflare API token](https://dash.cloudflare.com/profile/api-tokens) |
| CLOUDFLARE_ACCOUNT_ID    | String | Your account ID. Available in the URL at https://dash.cloudflare.com |
| CLOUDFLARE_TUNNEL_ID     | String | The ID of your Cloudflare tunnel                             |
| TRAEFIK_API_ENDPOINT     | String | The HTTP URI to Traefik's API (e.g., http://traefik:8080) |
| TRAEFIK_SERVICE_ENDPOINT | String | The HTTP URI to Traefik's web entrypoint (e.g., https://traefik:443) |

### Zone Configuration (Choose One)

#### Single Zone (Legacy)
| Environment Variable     | Type   | Description                                                  |
| :----------------------- | ------ | ------------------------------------------------------------ |
| CLOUDFLARE_ZONE_ID       | String | The Cloudflare zone ID of your site                         |
| DOMAIN_NAME              | String | (Optional) The domain name used for this zone               |

#### Multi-Zone (NEW)
| Environment Variable     | Type   | Description                                                  |
| :----------------------- | ------ | ------------------------------------------------------------ |
| CLOUDFLARE_ZONE_IDS      | String | Comma-separated list of Cloudflare zone IDs (e.g., `zone1,zone2,zone3`) |
| DOMAIN_NAMES             | String | Comma-separated list of domain names matching the zones (e.g., `example.com,test.com,demo.com`) |

**Note**: The order of zone IDs must match the order of domain names.

### Entrypoint Configuration (Choose One)

| Environment Variable     | Type   | Description                                                  |
| :----------------------- | ------ | ------------------------------------------------------------ |
| TRAEFIK_ENTRYPOINTS      | String | Comma-separated list of Traefik entrypoints (e.g., `web,websecure`) |
| TRAEFIK_ENTRYPOINT       | String | (Legacy) Single Traefik entrypoint (e.g., `web`)           |

### Optional Environment Variables

| Environment Variable | Type    | Default | Description                                                  |
| :------------------- | ------- | ------- | ------------------------------------------------------------ |
| SKIP_TLS_ROUTES      | Boolean | `true`  | Skip routes with TLS configured. Set to `false` to include TLS routes |
| POLL_INTERVAL        | String  | `10s`   | Polling interval (e.g., `10s`, `1m`, `30s`)                 |
| LOG_LEVEL            | String  | `info`  | Log level (`debug` or `info`)                               |
| IGNORE_PATTERNS      | String  | (empty) | Comma-separated regex patterns for domains to ignore (e.g., `^jellyfin\.,^media\.`) |
| ENABLE_DNS_CLEANUP   | Boolean | `true`  | Automatically remove DNS records for deleted resources       |

### Cloudflare Permissions

The `CLOUDFLARE_API_TOKEN` is your API token which can be created at: https://dash.cloudflare.com/profile/api-tokens

Ensure the permissions for your Cloudflare token match the following:

- Account -> Cloudflare Tunnel -> Edit
- Account -> Zero Trust -> Edit
- User -> User Details -> Read
- Zone -> DNS -> Edit

## Example with Pangolin

This example shows how to integrate Cloudflare tunnels with a Pangolin deployment.

1. First, set up Pangolin according to its [installation guide](https://docs.fossorial.io/Getting%20Started/quick-install)

2. Create an `.env` file with your Cloudflare credentials:

```bash
cd example
cp .env.example .env
vi .env
```

3. Add this service to your existing Pangolin `docker-compose.yml`:

```yaml
name: pangolin
services:
  pangolin:
    image: fosrl/pangolin:1.1.0
    container_name: pangolin
    restart: unless-stopped
    volumes:
      - ./config:/app/config
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3001/api/v1/"]
      interval: "3s"
      timeout: "3s"
      retries: 5
    networks:
      - pangolin_network

  traefik:
    image: traefik:v3.3.3
    container_name: traefik
    restart: unless-stopped
    ports:
      - 443:443
      - 80:80
      - 8080:8080
    depends_on:
      pangolin:
        condition: service_healthy
    command:
      - --configFile=/etc/traefik/traefik_config.yml
    environment:
      - CLOUDFLARE_DNS_API_TOKEN=your_dns_api_token_here
    volumes:
      - ./config/traefik:/etc/traefik:ro
      - ./config/letsencrypt:/letsencrypt
      - ./config/traefik/logs:/var/log/traefik
    networks:
      - pangolin_network

  cloudflared:
    image: cloudflare/cloudflared:2025.4.0
    container_name: cloudflared
    restart: unless-stopped
    command:
      - tunnel
      - --no-autoupdate
      - run
      - --token=your_cloudflared_token_here
    networks:
      - pangolin_network
    depends_on:
      - traefik

  traefik-cloudflare-tunnel:
    image: "hhftechnology/pangolin-cloudflare-tunnel:latest"
    container_name: pangolin-cloudflare-tunnel
    restart: unless-stopped
    environment:
      # Required Configuration
      - CLOUDFLARE_API_TOKEN=your_api_token_here
      - CLOUDFLARE_ACCOUNT_ID=your_account_id_here
      - CLOUDFLARE_TUNNEL_ID=your_tunnel_id_here
      - TRAEFIK_SERVICE_ENDPOINT=https://traefik:443
      - TRAEFIK_API_ENDPOINT=http://traefik:8080
      - TRAEFIK_ENTRYPOINTS=web,websecure

      # Multi-Zone Configuration (NEW)
      - CLOUDFLARE_ZONE_IDS=zone_id_1,zone_id_2
      - DOMAIN_NAMES=example.com,test.com

      # Optional Configuration
      - POLL_INTERVAL=10s
      - SKIP_TLS_ROUTES=false
      - LOG_LEVEL=debug
      - ENABLE_DNS_CLEANUP=true

      # Resource Exclusion (NEW) - Ignore Jellyfin and media services
      - IGNORE_PATTERNS=^jellyfin\.,^media\.
    networks:
      - pangolin_network
    depends_on:
      - traefik
      - cloudflared

networks:
  pangolin_network:
    driver: bridge
    name: pangolin_network
```

4. Restart your Pangolin stack:

```bash
sudo docker compose up -d
```

5. Create resources in Pangolin as usual. Resources with the specified entrypoint will be automatically exposed through Cloudflare tunnels.

## Use Cases

### Multi-Domain Setup

Manage multiple domains across different Cloudflare zones:

```bash
CLOUDFLARE_ZONE_IDS=zone1,zone2,zone3
DOMAIN_NAMES=example.com,test.com,demo.com
```

### Resource Exclusion

Exclude specific services that shouldn't use Cloudflare CDN (e.g., Jellyfin for TOS compliance):

```bash
IGNORE_PATTERNS=^jellyfin\.,^media\.,^plex\.
```

This will exclude:
- `jellyfin.example.com`
- `media.example.com`
- `plex.example.com`

### Automatic Cleanup

Enable automatic DNS cleanup to remove records for deleted resources:

```bash
ENABLE_DNS_CLEANUP=true
```

When a resource is deleted from Traefik, its corresponding DNS record will be automatically removed from Cloudflare.

## Architecture

The application is structured with a clean, modular architecture:

```
.
├── main.go                     # Application entry point
├── internal/
│   ├── config/                 # Configuration management
│   ├── traefik/                # Traefik API client and router management
│   ├── cloudflare/             # Cloudflare API client and operations
│   ├── sync/                   # Synchronization orchestration
│   └── errors/                 # Custom error types
└── pkg/
    └── retry/                  # Retry utility with exponential backoff
```

## Advanced Configuration

### Multi-Host Setup with Gerbil

You can use this tool with multiple Docker hosts on the same hypervisor layer while keeping Gerbil in your setup. This allows connecting multiple LXC containers (each running Docker) to a single centralized Pangolin instance.

### Custom Polling Intervals

Adjust the polling interval based on your needs:

```bash
POLL_INTERVAL=30s  # Less frequent polling (lower resource usage)
POLL_INTERVAL=5s   # More frequent polling (faster updates)
```

### Debug Logging

Enable debug logging for troubleshooting:

```bash
LOG_LEVEL=debug
```

## Troubleshooting

### DNS Records Not Created

1. Check that your `CLOUDFLARE_API_TOKEN` has the correct permissions
2. Verify that `CLOUDFLARE_ZONE_IDS` and `DOMAIN_NAMES` match correctly
3. Enable debug logging to see detailed error messages

### Domain Not Matching Zone

If you see warnings like "no matching zone", ensure:
- The domain is a subdomain of one of your configured `DOMAIN_NAMES`
- The order of `CLOUDFLARE_ZONE_IDS` matches `DOMAIN_NAMES`

### Resources Not Excluded

If the `IGNORE_PATTERNS` aren't working:
- Check that your regex patterns are correct
- Test patterns at https://regex101.com
- Remember to escape special characters (e.g., `\.` for literal dots)

## For More Information

- [Pangolin Documentation](https://docs.fossorial.io/Pangolin/)
- [Cloudflare Tunnel Documentation](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/)
- [Traefik Documentation](https://doc.traefik.io/traefik/)

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

## License

This project follows the MIT license.
