package collector

import (
	"fmt"
	"io"
)

// ReadAllStdin 读取全部数据，带大小限制
func ReadAllStdin(r io.Reader, limit int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if len(data) > limit {
		return nil, fmt.Errorf("stdin payload too large: exceeds %d bytes", limit)
	}
	return data, nil
}
