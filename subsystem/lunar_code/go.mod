module LunarSubsystem/LunarCode

go 1.26.0

require (
	LunarSubsystem/LunarHarness v0.0.0
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14
)

replace LunarSubsystem/LunarHarness => ../lunar_harness

replace LunarSubsystem/LunarDecoder => ../lunar_decoder

replace LunarSubsystem/EndpointDocs => ../endpoint_docs
