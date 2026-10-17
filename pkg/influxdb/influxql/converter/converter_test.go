package converter

import (
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

func TestMaybeFixValueFieldType(t *testing.T) {
	tests := []struct {
		name         string
		valueFields  data.Fields
		inputType    data.FieldType
		colIdx       int
		expectedType data.FieldType
	}{
		{
			name:         "should do nothing if both are the same type (bool)",
			valueFields:  data.Fields{data.NewFieldFromFieldType(data.FieldTypeNullableBool, 0)},
			inputType:    data.FieldTypeNullableBool,
			colIdx:       0,
			expectedType: data.FieldTypeNullableBool,
		},
		{
			name:         "should do nothing if both are the same type (string)",
			valueFields:  data.Fields{data.NewFieldFromFieldType(data.FieldTypeNullableString, 0)},
			inputType:    data.FieldTypeNullableString,
			colIdx:       0,
			expectedType: data.FieldTypeNullableString,
		},
		{
			name:         "should do nothing if both are the same type (float64)",
			valueFields:  data.Fields{data.NewFieldFromFieldType(data.FieldTypeNullableFloat64, 0)},
			inputType:    data.FieldTypeNullableFloat64,
			colIdx:       0,
			expectedType: data.FieldTypeNullableFloat64,
		},
		{
			name:         "should return nullableJson if both are nullableJson",
			valueFields:  data.Fields{data.NewFieldFromFieldType(data.FieldTypeNullableJSON, 0)}, //nolint:staticcheck
			inputType:    data.FieldTypeNullableJSON,                                             //nolint:staticcheck
			colIdx:       0,
			expectedType: data.FieldTypeNullableJSON, //nolint:staticcheck
		},
		{
			name:         "should return nullableString if valueField is nullableJson and input is nullableString",
			valueFields:  data.Fields{data.NewFieldFromFieldType(data.FieldTypeNullableJSON, 0)}, //nolint:staticcheck
			inputType:    data.FieldTypeNullableString,
			colIdx:       0,
			expectedType: data.FieldTypeNullableString,
		},
		{
			name:         "should return nullableBool if valueField is nullableJson and input is nullableBool",
			valueFields:  data.Fields{data.NewFieldFromFieldType(data.FieldTypeNullableJSON, 0)}, //nolint:staticcheck
			inputType:    data.FieldTypeNullableBool,
			colIdx:       0,
			expectedType: data.FieldTypeNullableBool,
		},
		{
			name:         "should return nullableFloat64 if valueField is nullableJson and input is nullableFloat64",
			valueFields:  data.Fields{data.NewFieldFromFieldType(data.FieldTypeNullableJSON, 0)}, //nolint:staticcheck
			inputType:    data.FieldTypeNullableFloat64,
			colIdx:       0,
			expectedType: data.FieldTypeNullableFloat64,
		},
		{
			name:         "should do nothing if valueField is different than nullableJson and input is anything but nullableJson",
			valueFields:  data.Fields{data.NewFieldFromFieldType(data.FieldTypeNullableFloat64, 0)},
			inputType:    data.FieldTypeNullableString,
			colIdx:       0,
			expectedType: data.FieldTypeNullableFloat64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maybeFixValueFieldType(tt.valueFields, tt.inputType, tt.colIdx)
			assert.Equal(t, tt.valueFields[tt.colIdx].Type(), tt.expectedType)
		})
	}
}

func TestEmptySeriesReturnsEmptyFrame(t *testing.T) {
	query := &models.Query{RawQuery: "SELECT value FROM cpu", ResultFormat: "time_series"}
	cases := []string{
		`{"results":[{"series":[{"name":"cpu","columns":["time","value"],"values":[]}]}]}`,
		`{"results":[{"series":[{"name":"cpu","columns":["time","value"]}]}]}`,
	}
	for _, body := range cases {
		iter := jsoniter.Parse(jsoniter.ConfigDefault, strings.NewReader(body), 1024)
		rsp := ReadInfluxQLStyleResult(iter, query)
		require.NotNil(t, rsp)
		require.NoError(t, rsp.Error)
		require.Len(t, rsp.Frames, 1)
		require.Len(t, rsp.Frames[0].Fields, 2)
		assert.Equal(t, 0, rsp.Frames[0].Fields[0].Len())
		assert.Equal(t, 0, rsp.Frames[0].Fields[1].Len())
	}

	iter := jsoniter.Parse(jsoniter.ConfigDefault, strings.NewReader(
		`{"results":[{"series":[{"name":"cpu","columns":["time","value"],"values":[[1,2]]}]}]}`,
	), 1024)
	rsp := ReadInfluxQLStyleResult(iter, query)
	require.NoError(t, rsp.Error)
	require.Len(t, rsp.Frames, 1)
	assert.Equal(t, 1, rsp.Frames[0].Fields[0].Len())
}

func TestMultipleSeriesKeepTheirPoints(t *testing.T) {
	query := &models.Query{RawQuery: "SELECT mean(value) FROM cpu", ResultFormat: "time_series"}
	body := `{"results":[{"series":[{"name":"cpu","tags":{"host":"server-a"},"columns":["time","mean"],"values":[[1000,42.5],[2000,40.1]]},{"name":"cpu","tags":{"host":"server-b"},"columns":["time","mean"],"values":[[1000,12.0]]}]}]}`
	iter := jsoniter.Parse(jsoniter.ConfigDefault, strings.NewReader(body), 1024)
	rsp := ReadInfluxQLStyleResult(iter, query)
	require.NoError(t, rsp.Error)
	require.Len(t, rsp.Frames, 2)
	require.Len(t, rsp.Frames[0].Fields, 2)
	require.Len(t, rsp.Frames[1].Fields, 2)
	assert.Equal(t, 2, rsp.Frames[0].Fields[0].Len())
	assert.Equal(t, 2, rsp.Frames[0].Fields[1].Len())
	assert.Equal(t, 1, rsp.Frames[1].Fields[0].Len())
	assert.Equal(t, 1, rsp.Frames[1].Fields[1].Len())
	assert.Equal(t, "server-a", rsp.Frames[0].Fields[1].Labels["host"])
	assert.Equal(t, "server-b", rsp.Frames[1].Fields[1].Labels["host"])
	assert.Equal(t, 42.5, *rsp.Frames[0].Fields[1].At(0).(*float64))
	assert.Equal(t, 12.0, *rsp.Frames[1].Fields[1].At(0).(*float64))
}
