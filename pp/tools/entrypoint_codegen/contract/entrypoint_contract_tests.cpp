#include "contract/entrypoint_contract.h"

#include <gtest/gtest.h>

namespace {

using epgen::facts::BridgeKind;
using epgen::facts::LayoutKind;
using epgen::facts::ParamRole;

TEST(EntrypointContractTest, RecognizesPromppFunctionPrefix) {
  EXPECT_TRUE(epgen::contract::is_entrypoint_function_name("prompp_fn"));
  EXPECT_FALSE(epgen::contract::is_entrypoint_function_name("other_fn"));
}

TEST(EntrypointContractTest, ClassifiesBridgeAnnotationNames) {
  EXPECT_EQ(epgen::contract::bridge_kind_for_annotation("prompp.entrypoint.cgo"), BridgeKind::kCGo);
  EXPECT_EQ(epgen::contract::bridge_kind_for_annotation("prompp.entrypoint.fastcgo"), BridgeKind::kFastCGo);
  EXPECT_EQ(epgen::contract::bridge_kind_for_annotation("other"), BridgeKind::kUnknown);
}

TEST(EntrypointContractTest, ClassifiesParameterNames) {
  EXPECT_EQ(epgen::contract::param_role_for_name("args"), ParamRole::kArgs);
  EXPECT_EQ(epgen::contract::param_role_for_name("res"), ParamRole::kRes);
  EXPECT_EQ(epgen::contract::param_role_for_name("other"), ParamRole::kOther);
}

TEST(EntrypointContractTest, ClassifiesLayoutNames) {
  EXPECT_EQ(epgen::contract::layout_kind_for_name("Arguments"), LayoutKind::kArguments);
  EXPECT_EQ(epgen::contract::layout_kind_for_name("Result"), LayoutKind::kResult);
  EXPECT_FALSE(epgen::contract::layout_kind_for_name("Other").has_value());
}

TEST(EntrypointContractTest, RecognizesVoidPointerSpellings) {
  EXPECT_TRUE(epgen::contract::is_void_pointer_type("void*"));
  EXPECT_TRUE(epgen::contract::is_void_pointer_type("void *"));
  EXPECT_FALSE(epgen::contract::is_void_pointer_type("const void*"));
}

}  // namespace
