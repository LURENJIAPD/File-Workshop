package objectstorage

import "context"

type DisabledClient struct{}

func NewDisabledClient() DisabledClient { return DisabledClient{} }

func (DisabledClient) Check(context.Context) error { return ErrDisabled }

func (DisabledClient) CreateMultipartUpload(context.Context, CreateMultipartUploadInput) (CreateMultipartUploadOutput, error) {
	return CreateMultipartUploadOutput{}, ErrDisabled
}

func (DisabledClient) PresignUploadPart(context.Context, PresignUploadPartInput) (PresignedRequest, error) {
	return PresignedRequest{}, ErrDisabled
}

func (DisabledClient) CompleteMultipartUpload(context.Context, CompleteMultipartUploadInput) error {
	return ErrDisabled
}

func (DisabledClient) AbortMultipartUpload(context.Context, AbortMultipartUploadInput) error {
	return ErrDisabled
}

func (DisabledClient) PresignGetObject(context.Context, PresignGetObjectInput) (PresignedRequest, error) {
	return PresignedRequest{}, ErrDisabled
}

func (DisabledClient) HeadObject(context.Context, HeadObjectInput) (HeadObjectOutput, error) {
	return HeadObjectOutput{}, ErrDisabled
}
