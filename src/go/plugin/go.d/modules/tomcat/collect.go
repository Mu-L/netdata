// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

var (
	urlPathServerStatus  = "/manager/status"
	urlQueryServerStatus = url.Values{"XML": {"true"}}.Encode()
)

func (t *Tomcat) collect() (map[string]int64, error) {
	mx, err := t.collectServerStatus()
	if err != nil {
		return nil, err
	}

	return mx, nil
}

func (t *Tomcat) collectServerStatus() (map[string]int64, error) {
	resp, err := t.queryServerStatus()
	if err != nil {
		return nil, err
	}

	if len(resp.Connectors) == 0 {
		return nil, errors.New("unexpected response: not tomcat server status data")
	}

	seenConns, seenPools := make(map[string]bool), make(map[string]bool)

	for i, v := range resp.Connectors {
		resp.Connectors[i].STMKey = cleanName(v.Name)
		ti := &resp.Connectors[i].ThreadInfo
		ti.CurrentThreadsIdle = ti.CurrentThreadCount - ti.CurrentThreadsBusy

		seenConns[v.Name] = true
		if !t.seenConnectors[v.Name] {
			t.seenConnectors[v.Name] = true
			t.addConnectorCharts(v.Name)
		}
	}

	for i, v := range resp.JVM.MemoryPools {
		resp.JVM.MemoryPools[i].STMKey = cleanName(v.Name)

		seenPools[v.Name] = true
		if !t.seenMemPools[v.Name] {
			t.seenMemPools[v.Name] = true
			t.addMemPoolCharts(v.Name, v.Type)
		}
	}

	for name := range t.seenConnectors {
		if !seenConns[name] {
			delete(t.seenConnectors, name)
			t.removeConnectorCharts(name)
		}
	}

	for name := range t.seenMemPools {
		if !seenPools[name] {
			delete(t.seenMemPools, name)
			t.removeMemoryPoolCharts(name)
		}
	}

	resp.JVM.Memory.Used = resp.JVM.Memory.Total - resp.JVM.Memory.Free

	return stm.ToMap(resp), nil
}

func cleanName(name string) string {
	r := strings.NewReplacer(" ", "_", ".", "_", "\"", "", "'", "")
	return strings.ToLower(r.Replace(name))
}

func (t *Tomcat) queryServerStatus() (*serverStatusResponse, error) {
	req, err := web.NewHTTPRequestWithPath(t.Request, urlPathServerStatus)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = urlQueryServerStatus

	var status serverStatusResponse

	if err := t.doOKDecode(req, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

func (t *Tomcat) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	if err := xml.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error decoding XML response from '%s': %v", req.URL, err)
	}

	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
