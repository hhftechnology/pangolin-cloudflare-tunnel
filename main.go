// Package main provides a service that automatically syncs Traefik router
// configurations to Cloudflare tunnels, including DNS record management.
// It polls the Traefik API periodically and updates Cloudflare tunnel
// configurations whenever changes are detected in the router configuration.
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-resty/resty/v2"
	"github.com/traefik/traefik/v3/pkg/muxer/http"

	log "github.com/sirupsen/logrus"
)

// Required environment variables:
// - CLOUDFLARE_API_TOKEN: API token for Cloudflare API access
// - CLOUDFLARE_ACCOUNT_ID: Cloudflare account ID
// - CLOUDFLARE_TUNNEL_ID: ID of the Cloudflare tunnel to update
// - CLOUDFLARE_ZONE_ID: Cloudflare zone ID for DNS records
// - TRAEFIK_API_ENDPOINT: Traefik API endpoint URL (e.g., http://localhost:8080)
// - TRAEFIK_ENTRYPOINTS: Comma-separated list of Traefik entrypoints to watch (e.g., "web,websecure")
//   OR
// - TRAEFIK_ENTRYPOINT: (Legacy) Single Traefik entrypoint to watch (e.g., "web")
// - TRAEFIK_SERVICE_ENDPOINT: Service endpoint for tunnel traffic
// 
// Optional environment variables:
// - SKIP_TLS_ROUTES: Set to "false" to include TLS-enabled routes in tunnel config (default: "true")
// - LOG_LEVEL: Set to "debug" for more verbose logging
// - POLL_INTERVAL: Interval between polls in seconds (default: "10s")

// Config holds application configuration loaded from environment variables
type Config struct {
	CloudflareToken        string
	CloudflareAccountID    string
	CloudflareTunnelID     string
	CloudflareZoneID       string
	TraefikAPIEndpoint     string
	TraefikEntrypoints     []string
	TraefikServiceEndpoint string
	SkipTLSRoutes          bool
	PollInterval           time.Duration
}

// TraefikRoutersResponse represents the response from polling Traefik routers
type TraefikRoutersResponse struct {
	Routers []Router
	Err     error
}

// Router represents a Traefik router configuration
type Router struct {
	EntryPoints []string `json:"entryPoints"`
	Service     string   `json:"service"`
	Rule        string   `json:"rule"`
	Status      string   `json:"status"`
	Using       []string `json:"using"`
	ServiceName string   `json:"name"`
	Provider    string   `json:"provider"`
	Middlewares []string `json:"middlewares,omitempty"`
	TLS         struct {
		CertResolver string   `json:"certResolver"`
		Options      string   `json:"options"`
		Domains      []string `json:"domains,omitempty"`
	} `json:"tls,omitempty"`
	Priority int `json:"priority,omitempty"`
}

func init() {
	// Configure logging
	log.SetFormatter(&log.TextFormatter{
		DisableColors: true,
		FullTimestamp: true,
	})
	
	// Set log level based on environment variable
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "debug" {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}

// loadConfig loads configuration from environment variables
func loadConfig() (*Config, error) {
	// Parse TraefikEntrypoints from comma-separated list
	entrypointsStr := os.Getenv("TRAEFIK_ENTRYPOINTS")
	var entrypoints []string
	if entrypointsStr != "" {
		for _, ep := range strings.Split(entrypointsStr, ",") {
			entrypoints = append(entrypoints, strings.TrimSpace(ep))
		}
	} else {
		// Backward compatibility for TRAEFIK_ENTRYPOINT
		singleEntrypoint := os.Getenv("TRAEFIK_ENTRYPOINT")
		if singleEntrypoint != "" {
			entrypoints = append(entrypoints, singleEntrypoint)
		}
	}
	
	// Parse SkipTLSRoutes (default to true for backward compatibility)
	skipTLSRoutes := true
	skipTLSStr := os.Getenv("SKIP_TLS_ROUTES")
	if skipTLSStr != "" {
		var err error
		skipTLSRoutes, err = strconv.ParseBool(skipTLSStr)
		if err != nil {
			log.Warnf("Invalid SKIP_TLS_ROUTES value: %s, defaulting to true", skipTLSStr)
		}
	}
	
	// Parse poll interval
	pollInterval := 10 * time.Second
	pollIntervalStr := os.Getenv("POLL_INTERVAL")
	if pollIntervalStr != "" {
		var err error
		pollInterval, err = time.ParseDuration(pollIntervalStr)
		if err != nil {
			log.Warnf("Invalid POLL_INTERVAL value: %s, defaulting to 10s", pollIntervalStr)
			pollInterval = 10 * time.Second
		}
	}
	
	config := &Config{
		CloudflareToken:        os.Getenv("CLOUDFLARE_API_TOKEN"),
		CloudflareAccountID:    os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
		CloudflareTunnelID:     os.Getenv("CLOUDFLARE_TUNNEL_ID"),
		CloudflareZoneID:       os.Getenv("CLOUDFLARE_ZONE_ID"),
		TraefikAPIEndpoint:     os.Getenv("TRAEFIK_API_ENDPOINT"),
		TraefikEntrypoints:     entrypoints,
		TraefikServiceEndpoint: os.Getenv("TRAEFIK_SERVICE_ENDPOINT"),
		SkipTLSRoutes:          skipTLSRoutes,
		PollInterval:           pollInterval,
	}

	// Validate required configuration
	missing := []string{}
	if config.CloudflareToken == "" {
		missing = append(missing, "CLOUDFLARE_API_TOKEN")
	}
	if config.CloudflareAccountID == "" {
		missing = append(missing, "CLOUDFLARE_ACCOUNT_ID")
	}
	if config.CloudflareTunnelID == "" {
		missing = append(missing, "CLOUDFLARE_TUNNEL_ID")
	}
	if config.CloudflareZoneID == "" {
		missing = append(missing, "CLOUDFLARE_ZONE_ID")
	}
	if config.TraefikAPIEndpoint == "" {
		missing = append(missing, "TRAEFIK_API_ENDPOINT")
	}
	if len(config.TraefikEntrypoints) == 0 {
		missing = append(missing, "TRAEFIK_ENTRYPOINTS or TRAEFIK_ENTRYPOINT")
	}
	if config.TraefikServiceEndpoint == "" {
		missing = append(missing, "TRAEFIK_SERVICE_ENDPOINT")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	return config, nil
}

// setupCloudflare initializes the Cloudflare API client
func setupCloudflare(config *Config) (*cloudflare.API, error) {
	cloudflareClient, err := cloudflare.NewWithAPIToken(config.CloudflareToken)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Cloudflare API client: %w", err)
	}
	return cloudflareClient, nil
}

// setupTraefikClient initializes the Traefik API client
func setupTraefikClient(config *Config) *resty.Client {
	return resty.New().SetBaseURL(config.TraefikAPIEndpoint)
}

func main() {
	// Load configuration
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set up Cloudflare client
	cloudflareClient, err := setupCloudflare(config)
	if err != nil {
		log.Fatalf("Failed to setup Cloudflare client: %v", err)
	}

	// Set up Traefik client
	traefikClient := setupTraefikClient(config)

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signalCh
		log.Infof("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Start main processing loop
	if err := runSyncLoop(ctx, config, cloudflareClient, traefikClient); err != nil {
		log.Fatalf("Error in sync loop: %v", err)
	}
}

// runSyncLoop polls Traefik for router changes and syncs them to Cloudflare
func runSyncLoop(ctx context.Context, config *Config, cloudflareClient *cloudflare.API, traefikClient *resty.Client) error {
	pollCh := pollTraefikRouters(ctx, traefikClient, config.PollInterval)
	var cache []Router

	for {
		select {
		case <-ctx.Done():
			return nil
		case poll, ok := <-pollCh:
			if !ok {
				return fmt.Errorf("poll channel closed unexpectedly")
			}
			
			if poll.Err != nil {
				log.Errorf("Error polling Traefik routers: %v", poll.Err)
				continue
			}

			// Skip if no changes to traefik routers
			if reflect.DeepEqual(cache, poll.Routers) {
				continue
			}

			log.Info("Changes detected in Traefik routers")

			// Update the cache
			cache = poll.Routers

			if err := processRouterChanges(ctx, config, cloudflareClient, poll.Routers); err != nil {
				log.Errorf("Failed to process router changes: %v", err)
				// Continue the loop instead of failing completely
			}
		}
	}
}

// processRouterChanges processes Traefik router changes and updates Cloudflare tunnel configuration
func processRouterChanges(ctx context.Context, config *Config, cloudflareClient *cloudflare.API, routers []Router) error {
	ingress, domains, err := buildIngressRules(routers, config)
	if err != nil {
		return fmt.Errorf("failed to build ingress rules: %w", err)
	}

	// Update tunnel configuration
	if err := syncTunnelConfig(ctx, cloudflareClient, config, ingress); err != nil {
		return fmt.Errorf("failed to sync tunnel configuration: %w", err)
	}

	// Update DNS records
	if err := syncDNSRecords(ctx, cloudflareClient, config, domains); err != nil {
		return fmt.Errorf("failed to sync DNS records: %w", err)
	}

	return nil
}

// buildIngressRules creates Cloudflare ingress rules from Traefik routers
func buildIngressRules(routers []Router, config *Config) ([]cloudflare.UnvalidatedIngressRule, []string, error) {
	ingress := []cloudflare.UnvalidatedIngressRule{}
	processedDomains := make(map[string]bool)
	domains := []string{}

	for _, router := range routers {
		// Skip disabled routes
		if router.Status != "enabled" {
			continue
		}

		// Skip routes with TLS configured if SkipTLSRoutes is enabled
		if config.SkipTLSRoutes && hasTLSEnabled(router) {
			log.WithField("router", router.ServiceName).Debug("Skipping TLS-enabled router")
			continue
		}

		// Only use routes with one of the specified entrypoints
		if !hasMatchingEntrypoint(router.EntryPoints, config.TraefikEntrypoints) {
			continue
		}

		// Parse domains from router rule
		routerDomains, err := http.ParseDomains(router.Rule)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse domains from rule %q: %w", router.Rule, err)
		}

		for _, domain := range routerDomains {
			// Skip duplicate domains
			if processedDomains[domain] {
				log.WithField("domain", domain).Info("Skipping duplicate domain")
				continue
			}
			
			processedDomains[domain] = true
			domains = append(domains, domain)

			log.WithFields(log.Fields{
				"domain":  domain,
				"service": config.TraefikServiceEndpoint,
			}).Info("Adding domain to tunnel configuration")

			// Create origin request config with TLS verification settings
			originRequest := &cloudflare.OriginRequestConfig{
				HTTPHostHeader: &domain,
			}
			
			// Add TLS verification settings
			noTLSVerify := true
			originRequest.NoTLSVerify = &noTLSVerify
			originRequest.OriginServerName = &domain

			ingress = append(ingress, cloudflare.UnvalidatedIngressRule{
				Hostname:       domain,
				Service:        config.TraefikServiceEndpoint,
				OriginRequest:  originRequest,
			})
		}
	}

	// Add catch-all rule
	ingress = append(ingress, cloudflare.UnvalidatedIngressRule{
		Service: "http_status:404",
	})

	return ingress, domains, nil
}

// hasTLSEnabled determines if a router has TLS properly configured
func hasTLSEnabled(router Router) bool {
	return router.TLS.CertResolver != "" && 
		   (router.TLS.Options != "" || len(router.TLS.Domains) > 0)
}

// syncTunnelConfig updates the Cloudflare tunnel configuration with new ingress rules
func syncTunnelConfig(ctx context.Context, cloudflareClient *cloudflare.API, config *Config, ingress []cloudflare.UnvalidatedIngressRule) error {
	return retryOperation(3, func() error {
		// Get Current tunnel config
		accountRC := cloudflare.AccountIdentifier(config.CloudflareAccountID)
		tunnelConfig, err := cloudflareClient.GetTunnelConfiguration(ctx, accountRC, config.CloudflareTunnelID)
		if err != nil {
			return fmt.Errorf("failed to get current tunnel configuration: %w", err)
		}

		// Update config with new ingress rules
		tunnelConfig.Config.Ingress = ingress
		_, err = cloudflareClient.UpdateTunnelConfiguration(ctx, accountRC, cloudflare.TunnelConfigurationParams{
			TunnelID: config.CloudflareTunnelID,
			Config:   tunnelConfig.Config,
		})
		if err != nil {
			return fmt.Errorf("failed to update tunnel configuration: %w", err)
		}

		log.Info("Tunnel configuration updated successfully")
		return nil
	})
}

// syncDNSRecords updates DNS records for all domains in the ingress rules
func syncDNSRecords(ctx context.Context, cloudflareClient *cloudflare.API, config *Config, domains []string) error {
	zoneIdentifier := cloudflare.ZoneIdentifier(config.CloudflareZoneID)
	tunnelDomain := fmt.Sprintf("%s.cfargotunnel.com", config.CloudflareTunnelID)
	
	for _, domain := range domains {
		if err := ensureDNSRecord(ctx, cloudflareClient, zoneIdentifier, domain, tunnelDomain); err != nil {
			log.WithError(err).Errorf("Failed to ensure DNS record for %s", domain)
			// Continue processing other domains
		}
	}
	
	// TODO: In a future version, implement cleanup of DNS records that are no longer in use
	
	return nil
}

// ensureDNSRecord ensures that a DNS record exists and is correctly configured
func ensureDNSRecord(ctx context.Context, cloudflareClient *cloudflare.API, zoneIdentifier *cloudflare.ResourceContainer, domain, tunnelDomain string) error {
	return retryOperation(3, func() error {
		// Create record template
		var proxied bool = true
		record := cloudflare.DNSRecord{
			Type:    "CNAME",
			Name:    domain,
			Content: tunnelDomain,
			TTL:     1,
			Proxied: &proxied,
		}

		// Check if record already exists
		existingRecords, _, err := cloudflareClient.ListDNSRecords(ctx, zoneIdentifier, cloudflare.ListDNSRecordsParams{Name: domain})
		if err != nil {
			return fmt.Errorf("error checking DNS records for %s: %w", domain, err)
		}

		// Create new record if it doesn't exist
		if len(existingRecords) == 0 {
			_, err := cloudflareClient.CreateDNSRecord(ctx, zoneIdentifier, cloudflare.CreateDNSRecordParams{
				Name:    record.Name,
				Type:    record.Type,
				Content: record.Content,
				TTL:     record.TTL,
				Proxied: record.Proxied,
			})
			if err != nil {
				return fmt.Errorf("failed to create DNS record for %s: %w", domain, err)
			}
			log.WithField("domain", domain).Info("DNS record created successfully")
			return nil
		}

		// Update record if content doesn't match
		if existingRecords[0].Content != tunnelDomain {
			_, err = cloudflareClient.UpdateDNSRecord(ctx, zoneIdentifier, cloudflare.UpdateDNSRecordParams{
				ID:      existingRecords[0].ID,
				Name:    record.Name,
				Type:    record.Type,
				Content: record.Content,
				TTL:     record.TTL,
				Proxied: record.Proxied,
			})
			if err != nil {
				return fmt.Errorf("failed to update DNS record for %s: %w", domain, err)
			}
			log.WithField("domain", domain).Info("DNS record updated successfully")
		}

		return nil
	})
}

// pollTraefikRouters periodically polls the Traefik API for router configurations.
// It returns a channel that emits router configurations whenever changes are detected.
// The polling interval is specified with a small random jitter added to avoid thundering herd.
func pollTraefikRouters(ctx context.Context, client *resty.Client, interval time.Duration) chan TraefikRoutersResponse {
	ch := make(chan TraefikRoutersResponse)
	
	go func() {
		defer close(ch)
		
		jitterSource := rand.New(rand.NewSource(time.Now().UnixNano()))
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var pollResponse TraefikRoutersResponse

				_, pollResponse.Err = client.R().
					EnableTrace().
					SetResult(&pollResponse.Routers).
					Get("/api/http/routers")

				if pollResponse.Err != nil {
					ch <- pollResponse
					// Don't break the loop on error, just report it and continue
					log.WithError(pollResponse.Err).Error("Failed to poll Traefik routers")
				} else {
					ch <- pollResponse
				}

				// Add jitter to avoid thundering herd
				jitter := time.Duration(jitterSource.Int63n(int64(interval) / 2))
				select {
				case <-ctx.Done():
					return
				case <-time.After(jitter):
					// This just adds a random delay
				}
			}
		}
	}()
	
	return ch
}

// hasMatchingEntrypoint checks if any of the router's entrypoints match our allowed list
func hasMatchingEntrypoint(routerEntrypoints []string, allowedEntrypoints []string) bool {
	// If no allowed entrypoints specified, accept all
	if len(allowedEntrypoints) == 0 {
		return true
	}
	
	for _, routerEP := range routerEntrypoints {
		for _, allowedEP := range allowedEntrypoints {
			if routerEP == allowedEP {
				return true
			}
		}
	}
	return false
}

// retryOperation attempts an operation multiple times with exponential backoff
func retryOperation(maxRetries int, operation func() error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = operation()
		if err == nil {
			return nil
		}
		
		if i < maxRetries-1 {
			backoffDuration := time.Duration(1<<i) * time.Second
			log.WithError(err).Warnf("Operation failed, retrying in %v (%d/%d)...", 
				backoffDuration, i+1, maxRetries)
			time.Sleep(backoffDuration)
		}
	}
	return fmt.Errorf("operation failed after %d retries: %w", maxRetries, err)
}