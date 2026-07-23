package httpadapter

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/urllifecycle"
)

type CachePolicy struct {
	Permanent time.Duration
	Temporary time.Duration
	Gone      time.Duration
	Unknown   time.Duration
}

type Options struct {
	TrustedOrigin     string
	Cache             CachePolicy
	UnknownAsNotFound bool
	OnError           func(http.ResponseWriter, *http.Request, error)
	Clock             func() time.Time
}

func Middleware(
	resolver urllifecycle.Resolver,
	options Options,
) (func(http.Handler) http.Handler, error) {
	if resolver == nil {
		return nil, fmt.Errorf("urllifecycle/httpadapter: resolver is required")
	}
	origin, err := parseOrigin(options.TrustedOrigin)
	if err != nil {
		return nil, err
	}
	if options.Cache.Permanent == 0 {
		options.Cache.Permanent = 24 * time.Hour
	}
	if options.Cache.Temporary == 0 {
		options.Cache.Temporary = 30 * time.Second
	}
	if options.Cache.Gone == 0 {
		options.Cache.Gone = time.Hour
	}
	if options.Cache.Unknown == 0 {
		options.Cache.Unknown = 10 * time.Second
	}
	if options.Cache.Permanent < 0 || options.Cache.Temporary < 0 ||
		options.Cache.Gone < 0 || options.Cache.Unknown < 0 {
		return nil, fmt.Errorf("urllifecycle/httpadapter: cache durations cannot be negative")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	onError := options.OnError
	if onError == nil {
		onError = func(writer http.ResponseWriter, _ *http.Request, _ error) {
			writer.Header().Set("Cache-Control", "no-store")
			http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		}
	}
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			resolution, err := resolver.Resolve(request.Context(), urllifecycle.Lookup{
				EscapedPath: request.URL.EscapedPath(),
				RawQuery:    request.URL.RawQuery,
			})
			if err != nil {
				onError(writer, request, err)
				return
			}
			switch resolution.Kind {
			case urllifecycle.ResolutionCanonical:
				next.ServeHTTP(writer, request)
			case urllifecycle.ResolutionUnknown:
				if !options.UnknownAsNotFound {
					next.ServeHTTP(writer, request)
					return
				}
				writer.Header().Set("Cache-Control", cacheControl(options.Cache.Unknown))
				http.NotFound(writer, request)
			case urllifecycle.ResolutionGone:
				writer.Header().Set("Cache-Control", cacheControl(options.Cache.Gone))
				writer.WriteHeader(http.StatusGone)
			case urllifecycle.ResolutionAlias, urllifecycle.ResolutionRedirect:
				location, err := safeLocation(origin, resolution.Location)
				if err != nil {
					onError(writer, request, err)
					return
				}
				ttl := options.Cache.Temporary
				if resolution.StatusCode == http.StatusMovedPermanently ||
					resolution.StatusCode == http.StatusPermanentRedirect {
					ttl = options.Cache.Permanent
				} else if resolution.ExpiresAt != nil {
					remaining := resolution.ExpiresAt.Sub(clock().UTC())
					if remaining < 0 {
						remaining = 0
					}
					if remaining < ttl {
						ttl = remaining
					}
				}
				writer.Header().Set("Location", location)
				writer.Header().Set("Cache-Control", cacheControl(ttl))
				writer.WriteHeader(resolution.StatusCode)
			default:
				onError(writer, request, fmt.Errorf("urllifecycle/httpadapter: unknown resolution %q", resolution.Kind))
			}
		})
	}, nil
}

func parseOrigin(raw string) (*url.URL, error) {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Scheme == "" || value.Hostname() == "" ||
		value.User != nil || value.RawQuery != "" || value.Fragment != "" ||
		(value.Path != "" && value.Path != "/") {
		return nil, fmt.Errorf("urllifecycle/httpadapter: trusted origin is invalid")
	}
	if value.Scheme != "https" && value.Scheme != "http" {
		return nil, fmt.Errorf("urllifecycle/httpadapter: trusted origin must use http or https")
	}
	value.Path = ""
	return value, nil
}

func safeLocation(origin *url.URL, raw string) (string, error) {
	value, err := url.Parse(raw)
	if err != nil || value.User != nil || value.Fragment != "" {
		return "", fmt.Errorf("urllifecycle/httpadapter: resolver returned an invalid Location")
	}
	if value.IsAbs() {
		if value.Scheme != "https" && value.Scheme != "http" {
			return "", fmt.Errorf("urllifecycle/httpadapter: resolver returned an unsafe Location")
		}
		return value.String(), nil
	}
	if value.Host != "" || !strings.HasPrefix(value.Path, "/") || strings.HasPrefix(value.Path, "//") {
		return "", fmt.Errorf("urllifecycle/httpadapter: resolver returned an unsafe local Location")
	}
	target := *origin
	target.Path = value.Path
	target.RawPath = value.RawPath
	target.RawQuery = value.RawQuery
	return target.String(), nil
}

func cacheControl(ttl time.Duration) string {
	if ttl <= 0 {
		return "no-store"
	}
	return "public, max-age=" + strconv.FormatInt(int64(ttl/time.Second), 10)
}
