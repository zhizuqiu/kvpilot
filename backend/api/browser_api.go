//go:build web

package api

import (
	"net/http"
	"tinyrdm/backend/services"
	"tinyrdm/backend/types"

	"github.com/gin-gonic/gin"
)

func registerBrowserRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/browser")

	g.POST("/open-connection", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().OpenConnection(req.Name))
	})

	g.POST("/close-connection", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().CloseConnection(req.Name))
	})

	g.POST("/open-database", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
			DB     int    `json:"db"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().OpenDatabase(req.Server, req.DB))
	})

	g.POST("/server-info", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ServerInfo(req.Name))
	})

	g.POST("/topology", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().Topology(req.Name))
	})

	g.POST("/acl/overview", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AclOverview(req.Name))
	})

	g.POST("/acl/user", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
			User string `json:"user"`
		}
		if c.ShouldBindJSON(&req) != nil || req.User == "" {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AclUser(req.Name, req.User))
	})

	g.POST("/acl/categories", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AclCategories(req.Name))
	})

	g.POST("/acl/setuser", func(c *gin.Context) {
		var req struct {
			Name  string   `json:"name"`
			User  string   `json:"user"`
			Rules []string `json:"rules"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AclSetUser(req.Name, req.User, req.Rules))
	})

	g.DELETE("/acl/user", func(c *gin.Context) {
		user := c.Query("user")
		if user == "" {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AclDeleteUser(c.Query("name"), user))
	})

	g.POST("/acl/log", func(c *gin.Context) {
		var req struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AclLog(req.Name, req.Count))
	})

	g.POST("/acl/dryrun", func(c *gin.Context) {
		var req struct {
			Name    string   `json:"name"`
			User    string   `json:"user"`
			Command []string `json:"command"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AclDryRun(req.Name, req.User, req.Command))
	})

	g.POST("/config/overview", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ConfigOverview(req.Name))
	})

	g.POST("/config/set", func(c *gin.Context) {
		var req struct {
			Name  string `json:"name"`
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ConfigSet(req.Name, req.Key, req.Value))
	})

	g.POST("/load-next-keys", func(c *gin.Context) {
		var req struct {
			Server     string `json:"server"`
			DB         int    `json:"db"`
			Match      string `json:"match"`
			KeyType    string `json:"keyType"`
			ExactMatch bool   `json:"exactMatch"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().LoadNextKeys(req.Server, req.DB, req.Match, req.KeyType, req.ExactMatch))
	})

	g.POST("/load-next-all-keys", func(c *gin.Context) {
		var req struct {
			Server     string `json:"server"`
			DB         int    `json:"db"`
			Match      string `json:"match"`
			KeyType    string `json:"keyType"`
			ExactMatch bool   `json:"exactMatch"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().LoadNextAllKeys(req.Server, req.DB, req.Match, req.KeyType, req.ExactMatch))
	})

	g.POST("/load-all-keys", func(c *gin.Context) {
		var req struct {
			Server     string `json:"server"`
			DB         int    `json:"db"`
			Match      string `json:"match"`
			KeyType    string `json:"keyType"`
			ExactMatch bool   `json:"exactMatch"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().LoadAllKeys(req.Server, req.DB, req.Match, req.KeyType, req.ExactMatch))
	})

	g.POST("/get-key-type", func(c *gin.Context) {
		var param types.KeySummaryParam
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetKeyType(param))
	})

	g.POST("/get-key-summary", func(c *gin.Context) {
		var param types.KeySummaryParam
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetKeySummary(param))
	})

	g.POST("/get-key-detail", func(c *gin.Context) {
		var param types.KeyDetailParam
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetKeyDetail(param))
	})

	g.POST("/convert-value", func(c *gin.Context) {
		var req struct {
			Value  any    `json:"value"`
			Decode string `json:"decode"`
			Format string `json:"format"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ConvertValue(req.Value, req.Decode, req.Format))
	})

	g.POST("/set-key-value", func(c *gin.Context) {
		var param types.SetKeyParam
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().SetKeyValue(param))
	})

	g.POST("/get-hash-value", func(c *gin.Context) {
		var param types.GetHashParam
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetHashValue(param))
	})

	g.POST("/set-hash-value", func(c *gin.Context) {
		var param types.SetHashParam
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().SetHashValue(param))
	})

	g.POST("/add-hash-field", func(c *gin.Context) {
		var req struct {
			Server     string `json:"server"`
			DB         int    `json:"db"`
			Key        any    `json:"key"`
			Action     int    `json:"action"`
			FieldItems []any  `json:"fieldItems"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AddHashField(req.Server, req.DB, req.Key, req.Action, req.FieldItems))
	})

	g.POST("/add-list-item", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
			DB     int    `json:"db"`
			Key    any    `json:"key"`
			Action int    `json:"action"`
			Items  []any  `json:"items"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AddListItem(req.Server, req.DB, req.Key, req.Action, req.Items))
	})

	g.POST("/set-list-item", func(c *gin.Context) {
		var param types.SetListParam
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().SetListItem(param))
	})

	g.POST("/set-set-item", func(c *gin.Context) {
		var req struct {
			Server  string `json:"server"`
			DB      int    `json:"db"`
			Key     any    `json:"key"`
			Remove  bool   `json:"remove"`
			Members []any  `json:"members"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().SetSetItem(req.Server, req.DB, req.Key, req.Remove, req.Members))
	})

	g.POST("/update-set-item", func(c *gin.Context) {
		var param types.SetSetParam
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().UpdateSetItem(param))
	})

	g.POST("/update-zset-value", func(c *gin.Context) {
		var param types.SetZSetParam
		if err := c.ShouldBindJSON(&param); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().UpdateZSetValue(param))
	})

	g.POST("/add-zset-value", func(c *gin.Context) {
		var req struct {
			Server     string             `json:"server"`
			DB         int                `json:"db"`
			Key        any                `json:"key"`
			Action     int                `json:"action"`
			ValueScore map[string]float64 `json:"valueScore"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AddZSetValue(req.Server, req.DB, req.Key, req.Action, req.ValueScore))
	})

	g.POST("/add-stream-value", func(c *gin.Context) {
		var req struct {
			Server     string `json:"server"`
			DB         int    `json:"db"`
			Key        any    `json:"key"`
			ID         string `json:"id"`
			FieldItems []any  `json:"fieldItems"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().AddStreamValue(req.Server, req.DB, req.Key, req.ID, req.FieldItems))
	})

	g.POST("/remove-stream-values", func(c *gin.Context) {
		var req struct {
			Server string   `json:"server"`
			DB     int      `json:"db"`
			Key    any      `json:"key"`
			IDs    []string `json:"ids"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().RemoveStreamValues(req.Server, req.DB, req.Key, req.IDs))
	})

	g.POST("/set-key-ttl", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
			DB     int    `json:"db"`
			Key    any    `json:"key"`
			TTL    int64  `json:"ttl"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().SetKeyTTL(req.Server, req.DB, req.Key, req.TTL))
	})

	g.POST("/batch-set-ttl", func(c *gin.Context) {
		var req struct {
			Server   string `json:"server"`
			DB       int    `json:"db"`
			Keys     []any  `json:"keys"`
			TTL      int64  `json:"ttl"`
			SerialNo string `json:"serialNo"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().BatchSetTTL(req.Server, req.DB, req.Keys, req.TTL, req.SerialNo))
	})

	g.POST("/delete-key", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
			DB     int    `json:"db"`
			Key    any    `json:"key"`
			Async  bool   `json:"async"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().DeleteKey(req.Server, req.DB, req.Key, req.Async))
	})

	g.POST("/delete-keys", func(c *gin.Context) {
		var req struct {
			Server   string `json:"server"`
			DB       int    `json:"db"`
			Keys     []any  `json:"keys"`
			SerialNo string `json:"serialNo"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().DeleteKeys(req.Server, req.DB, req.Keys, req.SerialNo))
	})

	g.POST("/delete-keys-by-pattern", func(c *gin.Context) {
		var req struct {
			Server  string `json:"server"`
			DB      int    `json:"db"`
			Pattern string `json:"pattern"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().DeleteKeysByPattern(req.Server, req.DB, req.Pattern))
	})

	g.POST("/rename-key", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
			DB     int    `json:"db"`
			Key    string `json:"key"`
			NewKey string `json:"newKey"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().RenameKey(req.Server, req.DB, req.Key, req.NewKey))
	})

	g.POST("/export-key", func(c *gin.Context) {
		var req struct {
			Server        string `json:"server"`
			DB            int    `json:"db"`
			Keys          []any  `json:"keys"`
			Path          string `json:"path"`
			IncludeExpire bool   `json:"includeExpire"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ExportKey(req.Server, req.DB, req.Keys, req.Path, req.IncludeExpire))
	})

	g.POST("/import-csv", func(c *gin.Context) {
		var req struct {
			Server   string `json:"server"`
			DB       int    `json:"db"`
			Path     string `json:"path"`
			Conflict int    `json:"conflict"`
			TTL      int64  `json:"ttl"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ImportCSV(req.Server, req.DB, req.Path, req.Conflict, req.TTL))
	})

	g.POST("/flush-db", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
			DB     int    `json:"db"`
			Async  bool   `json:"async"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().FlushDB(req.Server, req.DB, req.Async))
	})

	g.POST("/get-slow-logs", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
			Num    int64  `json:"num"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetSlowLogs(req.Server, req.Num))
	})

	g.POST("/get-client-list", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetClientList(req.Server))
	})

	g.POST("/list-sessions", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ListSessions(req.Server))
	})

	g.POST("/kill-session", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
			Node   string `json:"node"`
			ID     int64  `json:"id"`
			Addr   string `json:"addr"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().KillSession(req.Server, req.Node, req.ID, req.Addr))
	})

	g.POST("/start-bigkey-scan", func(c *gin.Context) {
		var req struct {
			Server      string `json:"server"`
			SampleSize  int    `json:"sampleSize"`
			ThresholdMB int    `json:"thresholdMB"`
			Full        bool   `json:"full"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().StartBigKeyScan(req.Server, req.SampleSize, req.ThresholdMB, req.Full))
	})

	g.POST("/get-bigkey-scan", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetBigKeyScan(req.Server))
	})

	g.POST("/cancel-bigkey-scan", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().CancelBigKeyScan(req.Server))
	})

	g.POST("/export-bigkeys", func(c *gin.Context) {
		var req struct {
			Path string                `json:"path"`
			Rows []services.BigKeyItem `json:"rows"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ExportBigKeys(req.Path, req.Rows))
	})

	g.POST("/get-keyspace-stats", func(c *gin.Context) {
		var req struct {
			Server     string `json:"server"`
			SampleSize int    `json:"sampleSize"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetKeyspaceStats(req.Server, req.SampleSize))
	})

	g.POST("/export-csv", func(c *gin.Context) {
		var req struct {
			Path   string     `json:"path"`
			Header []string   `json:"header"`
			Rows   [][]string `json:"rows"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ExportCSV(req.Path, req.Header, req.Rows))
	})

	g.POST("/clean-slow-logs", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().CleanSlowLogs(req.Server))
	})

	g.POST("/save-snapshot", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().SaveSnapshot(req.Server))
	})

	g.POST("/check-health", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().CheckHealth(req.Server))
	})

	g.POST("/slot-overview", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().SlotOverview(req.Server))
	})

	g.POST("/get-slot-distribution", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetSlotDistribution(req.Server))
	})

	g.POST("/scan-slot-distribution", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ScanSlotDistribution(req.Server))
	})

	g.POST("/preview-slot-migration", func(c *gin.Context) {
		var req struct {
			Server   string `json:"server"`
			SourceID string `json:"sourceId"`
			TargetID string `json:"targetId"`
			Slot     int    `json:"slot"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().PreviewSlotMigration(req.Server, req.SourceID, req.TargetID, req.Slot))
	})

	g.POST("/start-slot-migration", func(c *gin.Context) {
		var req struct {
			Server   string                        `json:"server"`
			SourceID string                        `json:"sourceId"`
			TargetID string                        `json:"targetId"`
			Slot     int                           `json:"slot"`
			Options  services.SlotMigrationOptions `json:"options"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().StartSlotMigration(req.Server, req.SourceID, req.TargetID, req.Slot, req.Options))
	})

	g.POST("/get-slot-migration", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().GetSlotMigration(req.Server))
	})

	g.POST("/pause-slot-migration", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().PauseSlotMigration(req.Server))
	})

	g.POST("/resume-slot-migration", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().ResumeSlotMigration(req.Server))
	})

	g.POST("/cancel-slot-migration", func(c *gin.Context) {
		var req struct {
			Server string `json:"server"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().CancelSlotMigration(req.Server))
	})

	g.POST("/slot-rebalance-plan", func(c *gin.Context) {
		var req struct {
			Server           string `json:"server"`
			Strategy         string `json:"strategy"`
			MaxSlots         int    `json:"maxSlots"`
			TolerancePercent int    `json:"tolerancePercent"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().SlotRebalancePlan(req.Server, req.Strategy, req.MaxSlots, req.TolerancePercent))
	})

	g.POST("/start-slot-rebalance", func(c *gin.Context) {
		var req struct {
			Server  string                        `json:"server"`
			Moves   []services.SlotMove           `json:"moves"`
			Options services.SlotMigrationOptions `json:"options"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		c.JSON(http.StatusOK, services.Browser().StartSlotRebalance(req.Server, req.Moves, req.Options))
	})

	g.POST("/get-cmd-history", func(c *gin.Context) {
		var req struct {
			PageNo   int `json:"pageNo"`
			PageSize int `json:"pageSize"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.JSResp{Msg: "invalid request"})
			return
		}
		if req.PageSize <= 0 {
			req.PageSize = 50
		}
		c.JSON(http.StatusOK, services.Browser().GetCmdHistory(req.PageNo, req.PageSize))
	})

	g.POST("/clean-cmd-history", func(c *gin.Context) {
		c.JSON(http.StatusOK, services.Browser().CleanCmdHistory())
	})
}
