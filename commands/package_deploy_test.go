package commands

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseParametersFromArgument(t *testing.T) {
	testCases := []struct {
		inputStr string
		valid    bool
		params   map[string]string
	}{
		{
			inputStr: "vmSize=Standard_DS2",
			valid:    true,
			params: map[string]string{
				"vmSize": "Standard_DS2",
			},
		},
		{
			inputStr: "vmSize=Standard_DS2,key=value",
			valid:    true,
			params: map[string]string{
				"vmSize": "Standard_DS2",
				"key":    "value",
			},
		},
		{
			inputStr: "vmSize=Standard_DS2,key=value,what=is",
			valid:    true,
			params: map[string]string{
				"vmSize": "Standard_DS2",
				"key":    "value",
				"what":   "is",
			},
		},
		{
			inputStr: "vm",
			valid:    false,
		},
		{
			inputStr: "vm=",
			valid:    false,
		},
		{
			inputStr: "=",
			valid:    false,
		},
		{
			inputStr: "=what",
			valid:    false,
		},
		{
			inputStr: "vmSize=key,",
			valid:    false,
		},
		{
			inputStr: "vmSize=key,what",
			valid:    false,
		},
		{
			inputStr: "vmSize=key,what=",
			valid:    false,
		},
	}

	for _, tc := range testCases {
		params, err := parseParametersFromArgument(tc.inputStr)
		if tc.valid {
			require.NoError(t, err)

			if len(params) != len(tc.params) {
				t.Fatalf("length of parsed params (%d) is not the same as input params (%d)", len(params), len(tc.params))
			}

			for key, value := range params {
				tcValue, ok := tc.params[key]
				require.True(t, ok)
				require.Equal(t, tcValue, value)
			}
		} else {
			require.Error(t, err)
		}
	}
}
