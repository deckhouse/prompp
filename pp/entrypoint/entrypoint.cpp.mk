amd64_headers := $(patsubst %.h, init/%_amd64.h, $(headers))
arm64_headers := $(patsubst %.h, init/%_arm64.h, $(headers))

.PRECIOUS: init/entrypoint.cpp
init/entrypoint.cpp: ## Built runtime entrypoint with flavor determine init function
init/entrypoint.cpp: init/entrypoint.amd64_includes | $(amd64_headers)
init/entrypoint.cpp: init/entrypoint.arm64_includes | $(arm64_headers)
init/entrypoint.cpp: init/entrypoint.ifuncs
init/entrypoint.cpp: entrypoint.cpp.template
	@while IFS="" read -r p || [ -n "$$p" ]; do \
		case $$p in \
		//#*) cat init/$$(echo "$$p" | $(sed) 's@^//#@@');; \
		*) printf '%s\n' "$$p";; \
		esac; \
	done < $< > $@
	@rm $(filter init/%, $^)

define prefix_functions
	echo "// This file is generated. DO NOT EDIT." > $@
	echo "" >> $@
	cat $^ | $(sed) '/^void /{h; $(patsubst %, g; s//void $(1)_%_/p;, $(2)) d}' >> $@
endef

.PRECIOUS: $(amd64_headers)
$(amd64_headers): init/%_amd64.h: %.h
	@mkdir -p ${@D}
	@$(call prefix_functions, amd64, $(amd64_escaped_flavors))

.PRECIOUS: $(arm64_headers)
$(arm64_headers): init/%_arm64.h: %.h
	@mkdir -p ${@D}
	@$(call prefix_functions, arm64, $(arm64_escaped_flavors))

.INTERMEDIATE: init/entrypoint.%_includes
init/entrypoint.%_includes: $(headers)
	@mkdir -p ${@D}
	@for i in $(patsubst %.h, %, $^); do\
		printf '#include "%s_%s.h"\n' "$$i" "$*";\
	done > $@

.INTERMEDIATE: init/entrypoint.ifuncs
init/entrypoint.ifuncs: init/all.symbols
	@$(sed) 's/^void \([^ (]*\)(\(.*\));/DEFINE_IFUNC(\1, (\2));/' $^ > $@

.INTERMEDIATE: init/all.symbols
init/all.symbols: $(headers)
	@cat $^ | grep '^void ' > $@
