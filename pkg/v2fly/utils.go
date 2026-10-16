package v2fly

import (
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func IsNotFoundError(err error) bool {
	return err != nil && (status.Code(err) == codes.NotFound || strings.HasSuffix(status.Convert(err).Message(), " not found."))
}
func IsAlreadyExistsError(err error) bool {
	return err != nil && (status.Code(err) == codes.AlreadyExists || strings.HasSuffix(status.Convert(err).Message(), " already exists."))
}
