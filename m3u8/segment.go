package m3u8

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Media segment tags specified in RFC 8216 section 4.4.4.
const (
	tagSegmentDuration = "#EXTINF"
	tagByteRange       = "#EXT-X-BYTERANGE"
	tagDiscontinuity   = "#EXT-X-DISCONTINUITY"
	tagKey             = "#EXT-X-KEY"
	tagMap             = "#EXT-X-MAP"
	tagDateTime        = "#EXT-X-PROGRAM-DATE-TIME"
	tagGap             = "#EXT-X-GAP"
	tagBitrate         = "#EXT-X-BITRATE"
	tagPart            = "#EXT-X-PART"
	tagDateRange       = "#EXT-X-DATERANGE"
)

// parseSegment returns the next segment from items and the leading
// item which indicated the start of a segment.
func parseSegment(items chan item, leading item) (*Segment, error) {
	var seg Segment
	switch leading.typ {
	case itemTag:
		switch leading.val {
		case tagSegmentDuration:
			it := <-items
			dur, err := parseSegmentDuration(it)
			if err != nil {
				return nil, fmt.Errorf("parse segment duration: %w", err)
			}
			seg.Duration = dur
		case tagKey:
			key, err := parseKey(items)
			if err != nil {
				return nil, fmt.Errorf("parse key: %w", err)
			}
			seg.Key = &key
		default:
			return nil, fmt.Errorf("parse leading item %s: unsupported", leading)
		}
	}

	for it := range items {
		switch it.typ {
		case itemError:
			return nil, errors.New(it.val)
		case itemURL:
			seg.URI = it.val
			return &seg, nil
		case itemNewline:
			continue
		default:
			if it.typ != itemTag {
				return nil, fmt.Errorf("unexpected %s", it)
			}
		}

		switch it.val {
		case tagSegmentDuration:
			it = <-items
			dur, err := parseSegmentDuration(it)
			if err != nil {
				return nil, fmt.Errorf("parse segment duration: %w", err)
			}
			seg.Duration = dur
		case tagByteRange:
			it = <-items
			r, err := parseByteRange(it.val)
			if err != nil {
				return nil, fmt.Errorf("parse byte range: %w", err)
			}
			seg.Range = r
		case tagDiscontinuity:
			seg.Discontinuity = true
		case tagKey:
			key, err := parseKey(items)
			if err != nil {
				return nil, fmt.Errorf("parse key: %w", err)
			}
			seg.Key = &key
		default:
			return nil, fmt.Errorf("parsing %s unsupported", it)
		}
	}
	return nil, fmt.Errorf("no url")
}

func parseSegmentDuration(it item) (time.Duration, error) {
	if it.typ != itemAttrName && it.typ != itemNumber {
		return 0, fmt.Errorf("got %s: want attribute name or number", it)
	}
	// Some numbers can be converted straight to ints, e.g.:
	// 	10
	// 	10.000
	// Others need to be converted from floating point, e.g:
	// 	9.967
	// Try the easiest paths first.
	if !strings.Contains(it.val, ".") {
		i, err := strconv.Atoi(it.val)
		if err != nil {
			return 0, err
		}
		return time.Duration(i) * time.Second, nil
	}
	// 10.000
	before, after, _ := strings.Cut(it.val, ".")
	var allZeroes = true
	for r := range after {
		if r != '0' {
			allZeroes = false
		}
	}
	if allZeroes {
		i, err := strconv.Atoi(before)
		if err != nil {
			return 0, err
		}
		return time.Duration(i) * time.Second, nil
	}
	seconds, err := strconv.ParseFloat(it.val, 32)
	if err != nil {
		return 0, err
	}
	// precision based on a 90KHz clock.
	microseconds := seconds * 1e6
	return time.Duration(microseconds) * time.Microsecond, nil
}

func parseKey(items chan item) (Key, error) {
	var key Key
	for it := range items {
		switch it.typ {
		case itemError:
			return key, errors.New(it.val)
		case itemNewline:
			return key, nil
		case itemAttrName:
			v := <-items
			if v.typ != itemEquals {
				return key, fmt.Errorf("expected %q after %s, got %s", "=", it.typ, v)
			}
			switch it.val {
			case "METHOD":
				v = <-items
				key.Method = parseEncryptMethod(v.val)
				if key.Method == encryptMethodInvalid {
					return key, fmt.Errorf("bad encrypt method %q", v.val)
				}
			case "URI":
				v = <-items
				key.URI = strings.Trim(v.val, `"`)
			case "IV":
				v = <-items
				b, err := hex.DecodeString(strings.TrimPrefix(v.val, "0x"))
				if err != nil {
					return key, fmt.Errorf("parse initialisation vector: %w", err)
				}
				if len(b) != len(key.IV) {
					return key, fmt.Errorf("bad initialisation length %d, want %d", len(b), len(key.IV))
				}
				copy(key.IV[:], b)
			case "KEYFORMAT":
				v = <-items
				key.Format = strings.Trim(v.val, `"`)
			case "KEYFORMATVERSIONS":
				v = <-items
				ss := strings.Split(v.val, "/")
				key.FormatVersions = make([]uint32, len(ss))
				for i := range ss {
					n, err := strconv.Atoi(ss[i])
					if err != nil {
						return key, fmt.Errorf("parse key format version: %w", err)
					}
					key.FormatVersions[i] = uint32(n)
				}
			default:
				return key, fmt.Errorf("TODO %s", it.val)
			}
		}
	}
	return key, fmt.Errorf("TODO")
}

func writeSegments(w io.Writer, segments []Segment) (n int, err error) {
	for i, seg := range segments {
		b, err := seg.MarshalText()
		if err != nil {
			return n, fmt.Errorf("segment %d: %w", i, err)
		}
		nn, err := fmt.Fprintln(w, string(b))
		n += nn
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (seg *Segment) MarshalText() ([]byte, error) {
	if seg.URI == "" {
		return nil, fmt.Errorf("empty URI")
	}
	if seg.Duration == 0 {
		return nil, fmt.Errorf("zero duration")
	}
	var tags []string
	if seg.Discontinuity {
		tags = append(tags, tagDiscontinuity)
	}
	if seg.DateRange != nil {
		buf := &bytes.Buffer{}
		if err := writeDateRange(buf, seg.DateRange); err != nil {
			return nil, fmt.Errorf("write date range: %w", err)
		}
		tags = append(tags, buf.String())
	}
	if seg.Range != [2]int{0, 0} {
		if seg.Range[0] >= seg.Range[1] {
			return nil, fmt.Errorf("impossible range: offset (%d) must be smaller than next %d", seg.Range[0], seg.Range[1])
		}
		tags = append(tags, fmt.Sprintf("%s:%s", tagByteRange, seg.Range))
	}
	if seg.Key != nil {
		tags = append(tags, seg.Key.String())
	}
	if seg.Map != nil {
		tags = append(tags, seg.Map.String())
	}
	if !seg.DateTime.IsZero() {
		tags = append(tags, fmt.Sprintf("%s:%s", tagDateTime, seg.DateTime.Format(RFC3339Milli)))
	}
	us := seg.Duration / time.Microsecond
	// we do .03f for the same precision as test-streams.mux.dev.
	tags = append(tags, fmt.Sprintf("%s:%.03f", tagSegmentDuration, float32(us)/1e6))
	tags = append(tags, seg.URI)
	return []byte(strings.Join(tags, "\n")), nil
}
