package process

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	privatev1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	cardamomv1 "go.abhg.dev/cardamom/internal/gen/cardamom/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestProtobufContractsSeparatePublicApplyFromPrivateServices(t *testing.T) {
	publicFiles := make([]string, 0, 1)
	publicServices := 0
	protoregistry.GlobalFiles.RangeFilesByPackage("cardamom.v1", func(file protoreflect.FileDescriptor) bool {
		publicFiles = append(publicFiles, file.Path())
		publicServices += file.Services().Len()
		return true
	})
	assert.Equal(t, []string{"cardamom/v1/apply.proto"}, publicFiles)
	assert.Zero(t, publicServices)

	apply := cardamomv1.File_cardamom_v1_apply_proto.Messages()
	applyIssue := apply.ByName("ApplyIssue")
	require.NotNil(t, applyIssue)
	issueType := applyIssue.Fields().ByName("type")
	require.NotNil(t, issueType)
	assert.Equal(t, protoreflect.StringKind, issueType.Kind())
	assert.True(t, issueType.HasPresence())

	applyDocument := apply.ByName("ApplyDocument")
	require.NotNil(t, applyDocument)
	onExisting := applyDocument.Fields().ByName("on_existing")
	require.NotNil(t, onExisting)
	assert.Equal(t, protoreflect.StringKind, onExisting.Kind())

	applyReceiptEntry := apply.ByName("ApplyReceiptEntry")
	require.NotNil(t, applyReceiptEntry)
	action := applyReceiptEntry.Fields().ByName("action")
	require.NotNil(t, action)
	assert.Equal(t, protoreflect.StringKind, action.Kind())

	planning := privatev1.File_cardamom_private_v1_planning_proto
	documentRequest := planning.Messages().ByName("ApplyDocumentRequest")
	require.NotNil(t, documentRequest)
	assert.Equal(
		t,
		cardamomv1.File_cardamom_v1_apply_proto.Messages().ByName("ApplyDocument").FullName(),
		documentRequest.Fields().ByName("document").Message().FullName(),
	)
	response := planning.Messages().ByName("ApplyDocumentResponse")
	require.NotNil(t, response)
	assert.Equal(
		t,
		cardamomv1.File_cardamom_v1_apply_proto.Messages().ByName("ApplyReceipt").FullName(),
		response.Fields().ByName("receipt").Message().FullName(),
	)
}
