#include "contract/entrypoint_contract.h"

#include <gtest/gtest.h>

namespace {

using entrypoint_codegen::facts::BridgeKind;
using entrypoint_codegen::facts::LayoutKind;
using entrypoint_codegen::facts::ParamRole;

TEST(EntrypointContractTest, RecognizesPromppFunctionPrefix) {
  EXPECT_TRUE(entrypoint_codegen::contract::is_entrypoint_function_name("prompp_fn"));
  EXPECT_FALSE(entrypoint_codegen::contract::is_entrypoint_function_name("other_fn"));
}

TEST(EntrypointContractTest, ClassifiesBridgeAnnotationNames) {
  EXPECT_EQ(entrypoint_codegen::contract::bridge_kind_for_annotation("prompp.entrypoint.cgo"), BridgeKind::kCGo);
  EXPECT_EQ(entrypoint_codegen::contract::bridge_kind_for_annotation("prompp.entrypoint.fastcgo"), BridgeKind::kFastCGo);
  EXPECT_EQ(entrypoint_codegen::contract::bridge_kind_for_annotation("other"), BridgeKind::kUnknown);
}

TEST(EntrypointContractTest, ClassifiesParameterNames) {
  EXPECT_EQ(entrypoint_codegen::contract::param_role_for_name("args"), ParamRole::kArgs);
  EXPECT_EQ(entrypoint_codegen::contract::param_role_for_name("res"), ParamRole::kRes);
  EXPECT_EQ(entrypoint_codegen::contract::param_role_for_name("other"), ParamRole::kOther);
}

TEST(EntrypointContractTest, ClassifiesLayoutNames) {
  EXPECT_EQ(entrypoint_codegen::contract::layout_kind_for_name("Arguments"), LayoutKind::kArguments);
  EXPECT_EQ(entrypoint_codegen::contract::layout_kind_for_name("Result"), LayoutKind::kResult);
  EXPECT_FALSE(entrypoint_codegen::contract::layout_kind_for_name("Other").has_value());
}

TEST(EntrypointContractTest, RecognizesVoidPointerSpellings) {
  EXPECT_TRUE(entrypoint_codegen::contract::is_void_pointer_type("void*"));
  EXPECT_TRUE(entrypoint_codegen::contract::is_void_pointer_type("void *"));
  EXPECT_FALSE(entrypoint_codegen::contract::is_void_pointer_type("const void*"));
}

}  // namespace
