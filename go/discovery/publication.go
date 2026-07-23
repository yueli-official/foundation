package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"path"
	"sort"
	"strings"
)

func (module *Module) Publish(
	ctx context.Context,
	plan PublicationPlan,
	sources Sources,
	target PublishTarget,
) (manifest PublicationManifest, report Report, resultErr error) {
	if module == nil {
		return manifest, report, failure(ErrorConfiguration, "module_required", "", "module is required")
	}
	if target == nil {
		return manifest, report, failure(ErrorContract, "publish_target_required", "target", "is required")
	}
	transaction, err := target.Begin(ctx)
	if err != nil {
		return manifest, report, targetError("target_begin_failed", true, err)
	}
	if transaction == nil {
		return manifest, report, targetError("target_begin_failed", false, errors.New("target returned nil transaction"))
	}
	committed := false
	defer func() {
		if !committed {
			abortErr := transaction.Abort(context.WithoutCancel(ctx), resultErr)
			if abortErr != nil && resultErr == nil {
				resultErr = targetError("target_abort_failed", true, abortErr)
			}
		}
	}()
	manifest.ContractVersion = ContractVersion
	var sitemapRoute string
	if plan.Sitemap != nil {
		sitemapArtifacts, route, sitemapReport, err := module.publishSitemap(ctx, *plan.Sitemap, sources, transaction)
		report.Diagnostics = append(report.Diagnostics, sitemapReport.Diagnostics...)
		if err != nil {
			return manifest, report, err
		}
		manifest.Artifacts = append(manifest.Artifacts, sitemapArtifacts...)
		sitemapRoute = route
	}
	for index, feed := range plan.Feeds {
		artifact, feedReport, err := module.publishFeed(ctx, feed, sources, transaction)
		report.Diagnostics = append(report.Diagnostics, feedReport.Diagnostics...)
		if err != nil {
			if discoveryError, ok := err.(*Error); ok && discoveryError.Path == "" {
				discoveryError.Path = fmt.Sprintf("feeds.%d", index)
			}
			return manifest, report, err
		}
		manifest.Artifacts = append(manifest.Artifacts, artifact)
	}
	if plan.Robots != nil {
		robots := *plan.Robots
		if sitemapRoute != "" && len(robots.Sitemaps) == 0 {
			robots.Sitemaps = []string{sitemapRoute}
		}
		artifact, err := module.publishRobots(ctx, robots, transaction)
		if err != nil {
			return manifest, report, err
		}
		manifest.Artifacts = append(manifest.Artifacts, artifact)
	}
	sort.Slice(manifest.Artifacts, func(i, j int) bool {
		return manifest.Artifacts[i].Name < manifest.Artifacts[j].Name
	})
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		left, right := report.Diagnostics[i], report.Diagnostics[j]
		return left.Protocol+"\x00"+left.Path+"\x00"+left.Code <
			right.Protocol+"\x00"+right.Path+"\x00"+right.Code
	})
	if err := transaction.Commit(ctx, manifest); err != nil {
		return manifest, report, targetError("target_commit_failed", true, err)
	}
	committed = true
	return manifest, report, nil
}

func normalizeArtifactRoute(value, fallback, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if strings.HasPrefix(value, "/") {
		value = strings.TrimPrefix(value, "/")
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == "" || cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "\\") ||
		strings.ContainsAny(cleaned, "?#") {
		return "", failure(ErrorContract, "invalid_artifact_route", field, "must be a safe relative route")
	}
	return cleaned, nil
}

func (module *Module) source(id SourceID, sources Sources, field string) (CursorSource, error) {
	if id == "" {
		return nil, failure(ErrorContract, "source_id_required", field, "is required")
	}
	source := sources[id]
	if source == nil {
		return nil, failure(ErrorContract, "source_not_found", field, "source %q is not registered", id)
	}
	return source, nil
}

func (module *Module) scan(
	ctx context.Context,
	source CursorSource,
	maxRecords int,
	visit func(Record) error,
) error {
	var cursor Cursor
	var previousSortKey string
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return &Error{Kind: ErrorCancelled, Code: "publication_cancelled", Cause: err}
		}
		batch, err := source.Next(ctx, cursor, module.limits.MaxSourceBatch)
		if err != nil {
			return &Error{
				Kind: ErrorSource, Code: "source_read_failed",
				Retryable: true, Cause: err,
			}
		}
		if len(batch.Records) > module.limits.MaxSourceBatch {
			return failure(ErrorCapacity, "source_batch_limit", "source", "returned more than %d records", module.limits.MaxSourceBatch)
		}
		for _, record := range batch.Records {
			if record.SortKey == "" {
				return failure(ErrorContract, "source_sort_key_required", "record.sortKey", "is required")
			}
			if previousSortKey != "" && record.SortKey <= previousSortKey {
				return failure(ErrorConflict, "source_order_violation", "record.sortKey", "must be strictly increasing")
			}
			previousSortKey = record.SortKey
			total++
			if maxRecords > 0 && total > maxRecords {
				return nil
			}
			if err := visit(record); err != nil {
				return err
			}
		}
		if batch.Done {
			return nil
		}
		if batch.NextCursor == "" || batch.NextCursor == cursor {
			return failure(ErrorConflict, "source_cursor_not_advancing", "source.nextCursor", "must advance for a non-final batch")
		}
		if len(batch.Records) == 0 {
			return failure(ErrorConflict, "source_empty_nonfinal_batch", "source.records", "a non-final batch must contain records")
		}
		cursor = batch.NextCursor
	}
}

type measuredWriter struct {
	writer io.WriteCloser
	hash   hashWriter
	bytes  int64
}

type hashWriter struct {
	state io.Writer
	sum   interface{ Sum([]byte) []byte }
}

func newMeasuredWriter(writer io.WriteCloser) *measuredWriter {
	digest := sha256.New()
	return &measuredWriter{
		writer: writer,
		hash:   hashWriter{state: digest, sum: digest},
	}
}

func (writer *measuredWriter) Write(value []byte) (int, error) {
	count, err := writer.writer.Write(value)
	if count > 0 {
		_, _ = writer.hash.state.Write(value[:count])
		writer.bytes += int64(count)
	}
	return count, err
}

func (writer *measuredWriter) Close() error {
	return writer.writer.Close()
}

func (writer *measuredWriter) artifact(name, mediaType string) Artifact {
	return Artifact{
		Name: name, MediaType: mediaType, Bytes: writer.bytes,
		SHA256: hex.EncodeToString(writer.hash.sum.Sum(nil)),
	}
}

func createArtifact(
	ctx context.Context,
	transaction PublicationWriter,
	name string,
	mediaType string,
) (*measuredWriter, error) {
	raw, err := transaction.Create(ctx, name, mediaType)
	if err != nil {
		return nil, targetError("target_create_failed", true, err)
	}
	if raw == nil {
		return nil, targetError("target_create_failed", false, errors.New("target returned nil writer"))
	}
	return newMeasuredWriter(raw), nil
}

func writeString(writer io.Writer, value string) error {
	_, err := io.WriteString(writer, value)
	if err != nil {
		return &Error{
			Kind: ErrorEncoding, Code: "artifact_write_failed",
			Retryable: true, Cause: err,
		}
	}
	return nil
}

func xmlText(value string) string {
	return html.EscapeString(value)
}

func targetError(code string, retryable bool, cause error) error {
	return &Error{
		Kind: ErrorTarget, Code: code, Retryable: retryable, Cause: cause,
	}
}
