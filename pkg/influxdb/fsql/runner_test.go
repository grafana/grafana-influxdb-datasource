package fsql

import (
	"context"
	"errors"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/flight"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeFlightClient drives runQuery through its error paths without a Flight
// SQL server.
type fakeFlightClient struct {
	executeInfo *flight.FlightInfo
	executeErr  error
	doGetErr    error
}

func (f *fakeFlightClient) Execute(_ context.Context, _ string, _ ...grpc.CallOption) (*flight.FlightInfo, error) {
	return f.executeInfo, f.executeErr
}

func (f *fakeFlightClient) DoGetWithHeaderExtraction(_ context.Context, _ *flight.Ticket, _ ...grpc.CallOption) (*flightReader, error) {
	return nil, f.doGetErr
}

func TestRunQueryErrorPaths(t *testing.T) {
	oneEndpoint := &flight.FlightInfo{Endpoint: []*flight.FlightEndpoint{{}}}

	tests := []struct {
		name       string
		client     *fakeFlightClient
		wantStatus backend.Status
		wantSource backend.ErrorSource
		wantErr    string
	}{
		{
			name:       "invalid argument maps to bad request downstream",
			client:     &fakeFlightClient{executeErr: status.Error(codes.InvalidArgument, "bad sql")},
			wantStatus: backend.StatusBadRequest,
			wantSource: backend.ErrorSourceDownstream,
			wantErr:    "bad sql",
		},
		{
			name:       "permission denied maps to forbidden downstream",
			client:     &fakeFlightClient{executeErr: status.Error(codes.PermissionDenied, "no access")},
			wantStatus: backend.StatusForbidden,
			wantSource: backend.ErrorSourceDownstream,
			wantErr:    "no access",
		},
		{
			name:       "unauthenticated maps to unauthorized downstream",
			client:     &fakeFlightClient{executeErr: status.Error(codes.Unauthenticated, "who are you")},
			wantStatus: backend.StatusUnauthorized,
			wantSource: backend.ErrorSourceDownstream,
			wantErr:    "who are you",
		},
		{
			name:       "non-grpc execute error maps to internal",
			client:     &fakeFlightClient{executeErr: errors.New("wire fell out")},
			wantStatus: backend.StatusInternal,
			wantErr:    "wire fell out",
		},
		{
			name: "unexpected endpoint count is internal",
			client: &fakeFlightClient{executeInfo: &flight.FlightInfo{
				Endpoint: []*flight.FlightEndpoint{{}, {}},
			}},
			wantStatus: backend.StatusInternal,
			wantErr:    "unsupported endpoint count in response: 2",
		},
		{
			name:       "doget failure is internal",
			client:     &fakeFlightClient{executeInfo: oneEndpoint, doGetErr: errors.New("stream refused")},
			wantStatus: backend.StatusInternal,
			wantErr:    "stream refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, errResp := runQuery(context.Background(), tt.client, "select 1")

			require.Nil(t, reader)
			require.NotNil(t, errResp)
			require.Error(t, errResp.Error)
			require.Contains(t, errResp.Error.Error(), tt.wantErr)
			require.Equal(t, tt.wantStatus, errResp.Status)
			require.Equal(t, tt.wantSource, errResp.ErrorSource)
		})
	}
}
