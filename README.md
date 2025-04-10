# Pangolin-cloudflare-tunnel

A bridge between Traefik and Cloudflare Zero-Trust tunnels that enables Pangolin users to leverage Cloudflare's global network alongside WireGuard tunnels.

## Overview

This tool synchronizes Traefik routes with Cloudflare Zero-Trust tunnels, providing an alternative or complementary tunneling option for Pangolin deployments. While Pangolin uses WireGuard tunnels by default, this integration allows you to:

- Expose Pangolin-managed services through Cloudflare's global network
- Take advantage of Cloudflare's DDoS protection and caching capabilities
- Provide an alternative remote access method alongside Pangolin's WireGuard tunnels

## Integration with Pangolin

When used with Pangolin:

1. Pangolin manages your internal resources.
2. Traefik (used by Pangolin) handles the local routing
3. This tool synchronizes Traefik routes to Cloudflare tunnels
4. Cloudflare provides an additional layer of protection and global distribution

This creates a powerful combination where you can use Pangolin for secure local deployment via Cloudflare tunnels for public-facing services for unraid/NAS user with opening ports or buying VPS.

## Configuration

| Environment Variable     | Type   | Description                                                  |
| :----------------------- | ------ | ------------------------------------------------------------ |
| CLOUDFLARED_TOKEN        | String | Token for the `cloudflared` daemon. This is the token provided after [creating a tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/tunnel-guide/#1-create-a-tunnel). |
| CLOUDFLARE_API_TOKEN     | String | A valid [cloudflare API token](https://dash.cloudflare.com/profile/api-tokens) |
| CLOUDFLARE_ACCOUNT_ID    | String | Your account ID. Available in the URL at https://dash.cloudflare.com |
| CLOUDFLARE_TUNNEL_ID     | String | The ID of your cloudlfare tunnel                             |
| CLOUDFLARE_ZONE_ID       | String | The cloudflare zone ID of your site.                         |
| DOMAIN_NAME              | String | The domain name used for these tunnels                       |
| TRAEFIK_API_ENDPOINT     | String | The HTTP URI to Traefik's API ( http://traefik:8080) |
| TRAEFIK_SERVICE_ENDPOINT | String | The HTTP URI to Traefik's web entrypoint (https://traefik:443)                     |
| TRAEFIK_ENTRYPOINT       | String | Imp (web,websecure) |
| POLL_INTERVAL       | String | Imp (10s) |
| SKIP_TLS_ROUTES       | String | Imp (false) Include TLS-enabled routes |
| LOG_LEVEL       | String | Imp (debug) |

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
      - CLOUDFLARE_DNS_API_TOKEN=MDVq5cqxxqwiPe3lOFS9jW5Q10Xs9GOrOUB5
    volumes:
      - ./config/traefik:/etc/traefik:ro # Volume to store the Traefik configuration
      - ./config/letsencrypt:/letsencrypt # Volume to store the Let's Encrypt certificates
      - ./config/traefik/logs:/var/log/traefik # Volume to store Traefik logs
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
      - --token=UFCDLJePHRt1nTrMAKQ9RfeUw1iUyMqXcscMLiMygHdELmrxvzHwe74Jn2UiSteheLtPRD4sLO59alBrk3TdrCcbutPgCeV0JxWMrBkMd8G025qkQJoTONt7xZpIbAS0
    networks:
      - pangolin_network
    depends_on:
      - traefik  

  traefik-cloudflare-tunnel:
    image: "hhftechnology/pangolin-cloudflare-tunnel:latest"
    container_name: pangolin-cloudflare-tunnel
    restart: unless-stopped
    environment:
      - CLOUDFLARE_API_TOKEN=MDVq5cqxxqwiPe3lOFS9jW5Q10Xs9GOrOUB5
      - CLOUDFLARE_ACCOUNT_ID=xfzFks0EuhA0wTAfwpUmTSOFuNboyM7Pzhz
      - CLOUDFLARE_TUNNEL_ID=z8a6c73b-22a4-5ghu-ad91-f1acce880d1f
      - CLOUDFLARE_ZONE_ID=RSPqQ9eaySaMSmISLupfeN9eAhXZQ35Ckwj0wgU
      - TRAEFIK_SERVICE_ENDPOINT=https://traefik:443
      - TRAEFIK_API_ENDPOINT=http://traefik:8080
      - TRAEFIK_ENTRYPOINTS=web,websecure
      - POLL_INTERVAL=10s  # Added to configure polling interval
      - SKIP_TLS_ROUTES=false  # Include TLS-enabled routes
      - LOG_LEVEL=debug
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


## Advanced Configuration

For more complex setups and additional configuration options, please refer to:

- [Pangolin Documentation](https://docs.fossorial.io/Pangolin/)
- [Cloudflare Tunnel Documentation](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/)
- [Traefik Documentation](https://doc.traefik.io/traefik/)
