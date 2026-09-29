package dto

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// WriteCSV writes a header row plus data rows as a downloadable CSV response
// (platform upgrade Module 24: Reports and Exports). filenamePrefix gets
// today's date and ".csv" appended (e.g. "riders-wallet-report" ->
// "riders-wallet-report-20260929.csv"), mirroring restaurant-service's
// utils.WriteCSVResponse so both services produce exports the same way.
func WriteCSV(c *gin.Context, filenamePrefix string, header []string, rows [][]string) {
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)

	if err := w.Write(header); err != nil {
		InternalError(c, "failed to build csv")
		return
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			InternalError(c, "failed to build csv")
			return
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		InternalError(c, "failed to finalize csv")
		return
	}

	filename := filenamePrefix + "-" + time.Now().Format("20060102") + ".csv"
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}
