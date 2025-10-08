#pragma once

#include <ostream>
#include <sstream>
#include <string>
#include <string_view>
#include <vector>

#include <simdutf/simdutf.h>
#include <quasis_crypto/md5.hh>
#include "re2/re2.h"

#include "bare_bones/bit.h"
#include "bare_bones/exception.h"
#include "bare_bones/preprocess.h"
#include "primitives/go_slice.h"

namespace PromPP::Prometheus::Relabel {

// label_name_is_valid validate label name.
PROMPP_ALWAYS_INLINE bool label_name_is_valid(const std::string_view& name) {
  if (name.empty()) {
    return false;
  }

  if (!std::ranges::all_of(name.begin() + 1, name.end(), [](char c) PROMPP_LAMBDA_INLINE { return std::isalnum(c) || c == '_'; })) {
    return false;
  }

  if (!(std::isalpha(name[0]) || name[0] == '_')) {
    return false;
  }

  return true;
}

// label_value_is_valid validate label value.
PROMPP_ALWAYS_INLINE bool label_value_is_valid(const std::string_view& value) noexcept {
  return simdutf::validate_utf8(value.data(), value.length());
}

// metric_name_value_is_valid validate value for metric name(__name__).
PROMPP_ALWAYS_INLINE bool metric_name_value_is_valid(const std::string_view& value) {
  if (value.empty()) {
    return false;
  }

  if (!std::ranges::all_of(value.begin() + 1, value.end(), [](char c) PROMPP_LAMBDA_INLINE { return std::isalnum(c) || c == '_' || c == ':'; })) {
    return false;
  }

  if (!(std::isalpha(value[0]) || value[0] == '_' || value[0] == ':')) {
    return false;
  }

  return true;
}

// pPatternPartType - is the pattern part type.
enum pPatternPartType : uint8_t {
  // pNoType - unknown type, init state.
  pUnknownType = 0,
  // pGroup - regex id group.
  pGroup,
  // pSting - regex name group.
  pSting,
};

// PatternPart - dismantled pattern.
class PatternPart {
  pPatternPartType type_;
  union {
    std::string_view string_;
    int group_;
  } data_;

 public:
  PROMPP_ALWAYS_INLINE explicit PatternPart(std::string_view s) : type_(pSting), data_{.string_ = s} {}
  PROMPP_ALWAYS_INLINE explicit PatternPart(int g) : type_(pGroup), data_{.group_ = g} {}

  // write - convert parts to out.
  PROMPP_ALWAYS_INLINE void write(std::string& out, const std::vector<std::string_view>& groups) const {
    if (type_ == pGroup) {
      out += groups[data_.group_];
    } else {
      out += data_.string_;
    }
  }
};

// Regexp - wrapper on re2.
class Regexp {
  // use ptr because re2::RE2 move constructor is delete.
  std::unique_ptr<re2::RE2> re_;

 public:
  // Regexp - work without ("^(?:" + std::string(s) + ")$").
  PROMPP_ALWAYS_INLINE explicit Regexp(const std::string_view& s) noexcept : re_(std::make_unique<re2::RE2>(s)) {}

  // number_of_capturing_groups - return the number of capturing sub-patterns, or -1 if the regexp wasn't valid on construction. The overall match ($0) does not
  // count. Use in test.
  [[nodiscard]] PROMPP_ALWAYS_INLINE int number_of_capturing_groups() const { return re_->NumberOfCapturingGroups(); }

  // groups - get named capturing groups and number groups.
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::map<std::string, int> groups() const {
    // get named capturing groups
    std::map<std::string, int> named_groups = re_->NamedCapturingGroups();
    // add number groups to named capturing groups
    for (int i = 0; i <= number_of_capturing_groups(); ++i) {
      named_groups.emplace(std::to_string(i), i);
    }

    return named_groups;
  }

  // match_to_args - match expression and return result args.
  PROMPP_ALWAYS_INLINE bool match_to_args(std::string_view src, std::vector<std::string_view>& res) const {
    const int n = number_of_capturing_groups();

    // search full match to args, where size - number of capturing groups
    res.resize(n + 1);
    res[0] = src;
    std::vector<RE2::Arg> re_args;
    re_args.reserve(n);
    std::vector<RE2::Arg*> re_args_ptr;
    re_args_ptr.reserve(n);
    for (int i = 1; i <= n; ++i) {
      re_args.emplace_back(&res[i]);
      re_args_ptr.emplace_back(&re_args[i - 1]);
    }

    if (!RE2::FullMatchN(src, *re_, &re_args_ptr[0], n)) {
      res.clear();
      return false;
    }

    return true;
  }

  // replace_with_args - replace in template with incoming args.
  PROMPP_ALWAYS_INLINE static void replace_with_args(std::string& buf, const std::vector<std::string_view>& args, const std::vector<PatternPart>& tmpl) {
    buf.clear();

    if (tmpl.empty()) [[unlikely]] {
      // no template or source data
      return;
    }

    for (auto& val : tmpl) {
      val.write(buf, args);
    }
  }

  // replace_full - find match for source and replace in template.
  PROMPP_ALWAYS_INLINE void replace_full(std::string& out, std::string_view src, const std::vector<PatternPart>& tmpl) const {
    out.clear();

    if (src.empty() || tmpl.empty()) [[unlikely]] {
      // no template or source data
      return;
    }

    std::vector<std::string_view> res_args;
    if (!match_to_args(src, res_args)) {
      // no entries in regexp
      return;
    }

    replace_with_args(out, res_args, tmpl);
  }

  // full_match - check text for full match regexp.
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool full_match(std::string_view str) const { return RE2::FullMatch(str, *re_); }
};

struct GORelabelConfig {
  // source_labels - a list of labels from which values are taken and concatenated with the configured separator in order.
  Primitives::Go::SliceView<Primitives::Go::String> source_labels;
  // separator - is the string between concatenated values from the source labels.
  Primitives::Go::String separator;
  // regex - against which the concatenation is matched.
  Primitives::Go::String regex;
  // modulus - to take of the hash of concatenated values from the source labels.
  uint64_t modulus;
  // target_label - is the label to which the resulting string is written in a replacement.
  // Regexp interpolation is allowed for the replace action.
  Primitives::Go::String target_label;
  // replacement - is the regex replacement pattern to be used.
  Primitives::Go::String replacement;
  // action - is the action to be performed for the relabeling.
  uint8_t action;
};

// rAction - is the action to be performed on relabeling.
enum rAction : uint8_t {
  // NoAction - no action, init state.
  rNoAction = 0,
  // Drop - drops targets for which the input does match the regex.
  rDrop,
  // Keep - drops targets for which the input does not match the regex.
  rKeep,
  // DropEqual - drops targets for which the input does match the target.
  rDropEqual,
  // KeepEqual - drops targets for which the input does not match the target.
  rKeepEqual,
  // Replace - performs a regex replacement.
  rReplace,
  // Lowercase - maps input letters to their lower case.
  rLowercase,
  // Uppercase - maps input letters to their upper case.
  rUppercase,
  // HashMod - sets a label to the modulus of a hash of labels.
  rHashMod,
  // LabelMap - copies labels to other labelnames based on a regex.
  rLabelMap,
  // LabelDrop - drops any label matching the regex.
  rLabelDrop,
  // LabelKeep - drops any label not matching the regex.
  rLabelKeep,
};

// relabelStatus resulting relabeling status.
enum relabelStatus : uint8_t {
  // Drop the result should be dropped.
  rsDrop = 0,
  // Invalid the result was invalid.
  rsInvalid,
  // Keep the result should be keeped.
  rsKeep,
  // Relabel the result relabeled and should be keeped.
  rsRelabel,
};

// RelabelConfig - config for relabeling.
class RelabelConfig {
  // source_labels - a list of labels from which values are taken and concatenated with the configured separator in order.
  std::vector<std::string_view> source_labels_;
  // separator - is the string between concatenated values from the source labels.
  std::string_view separator_;
  // regexp - against which the concatenation is matched.
  Regexp regexp_;
  // modulus - to take of the hash of concatenated values from the source labels.
  uint64_t modulus_;
  // target_label - is the label to which the resulting string is written in a replacement.
  // Regexp interpolation is allowed for the replace action.
  std::string_view target_label_;
  // replacement - is the regex replacement pattern to be used.
  std::string_view replacement_;
  // action - is the action to be performed for the relabeling.
  rAction action_;
  // target_label_parts - dismantled target_label.
  std::vector<PatternPart> target_label_parts_;
  // replacement_parts - dismantled replacement.
  std::vector<PatternPart> replacement_parts_;

  // extract - extract from source letter or digit value.
  PROMPP_ALWAYS_INLINE static std::string extract(const re2::RE2& rgx_validate, const std::string_view& src) {
    std::string name;
    RE2::PartialMatch(src, rgx_validate, &name);
    return name;
  }

  // is_valid_name - validate source on letter or digit value.
  PROMPP_ALWAYS_INLINE static bool is_valid_name(const re2::RE2& rgx_validate, std::string_view src) { return RE2::FullMatch(src, rgx_validate); }

  // parse - parse template on parts.
  PROMPP_ALWAYS_INLINE static void parse(const Regexp& regexp, const re2::RE2& rgx_validate, std::string_view tmpl, std::vector<PatternPart>& src_parts) {
    std::map<std::string, int> groups = regexp.groups();
    auto p = std::string_view(tmpl);
    while (true) {
      if (p.empty()) {
        break;
      }
      // search '$' and cut before
      const size_t i = p.find('$');
      if (std::string_view substr_p = p.substr(0, i); !substr_p.empty()) {
        src_parts.emplace_back(substr_p);
      }
      if (i == std::string_view::npos) {
        break;
      }
      p.remove_prefix(i + 1);
      switch (p[0]) {
        // if contains '$$'
        case '$': {
          // "$"
          src_parts.emplace_back(tmpl.substr(tmpl.size() - p.size() - 1, 1));
          p.remove_prefix(1);
          continue;
        }
        // if contains '{...}'
        case '{': {
          p.remove_prefix(1);
          const size_t j = p.find('}');
          if (j == std::string_view::npos) {
            // if '}' not found cut - "${"
            src_parts.emplace_back(tmpl.substr(tmpl.size() - p.size() - 2, 2));
            continue;
          }

          std::string_view g_name = p.substr(0, j);
          if (auto rec = groups.find(std::string(g_name)); rec != groups.end()) {
            // if g_name found in map add as group(int)
            src_parts.emplace_back(rec->second);
            p.remove_prefix(g_name.size() + 1);
            continue;
          }

          if (!is_valid_name(rgx_validate, g_name)) {
            // if g_name invalid add as is - "${" + std::string{g_name} + "}"
            src_parts.emplace_back(tmpl.substr(tmpl.size() - p.size() - 2, g_name.size() + 3));
            p.remove_prefix(g_name.size() + 1);
            continue;
          }

          // if g_name not found in map and g_name valid - cut g_name
          p.remove_prefix(g_name.size() + 1);

          continue;
        }

        default: {
          // search '$' and extract g_name
          const auto j = p.find('$');
          std::string_view g_name = p.substr(0, j);
          std::string name = extract(rgx_validate, g_name);
          if (name.empty()) {
            // if name invalid add as is - "$"
            src_parts.emplace_back(tmpl.substr(tmpl.size() - p.size() - 1, 1));
            continue;
          }
          auto rec = groups.find(name);
          std::string_view substr_g_name = g_name.substr(name.size(), g_name.size());
          if (rec != groups.end()) {
            // if g_name found in map add as group(int)
            src_parts.emplace_back(rec->second);
            if (!substr_g_name.empty()) {
              src_parts.emplace_back(substr_g_name);
            }
            p.remove_prefix(g_name.size());
            continue;
          }

          // if g_name not found in map cut g_name
          if (!substr_g_name.empty()) {
            src_parts.emplace_back(substr_g_name);
          }
          p.remove_prefix(g_name.size());
        }
      }
    }
  }

  // make_hash_uint64 - make uint64 from md5 hash.
  static PROMPP_ALWAYS_INLINE uint64_t make_hash_uint64(const std::string& src) {
    crypto::MD5<> hash;
    hash.update(src.c_str(), src.size());
    const auto& digest = hash.digest();

    // Use only the last 8 bytes of the hash to give the same result as earlier versions of prom code.
    static constexpr auto shift = sizeof(uint64_t);
    // need return BigEndian
    return BareBones::Bit::be(*reinterpret_cast<const uint64_t*>(&digest[shift]));
  }

  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE std::string get_value(LabelsBuilder& builder) const {
    std::string value;

    for (size_t i = 0; i < source_labels_.size(); ++i) {
      const auto lv = builder.get(source_labels_[i]);
      if (i == 0) [[unlikely]] {
        value += lv;
        continue;
      }
      value += separator_;
      value += lv;
    }

    return value;
  }

 public:
  // RelabelConfig - constructor for RelabelConfig from go-config.
  template <class GORelabelConfig>
  PROMPP_ALWAYS_INLINE explicit RelabelConfig(GORelabelConfig* go_rc) noexcept
      : separator_{static_cast<std::string_view>(go_rc->separator)},
        regexp_(static_cast<std::string_view>(go_rc->regex)),
        modulus_{go_rc->modulus},
        target_label_{static_cast<std::string_view>(go_rc->target_label)},
        replacement_{static_cast<std::string_view>(go_rc->replacement)},
        action_{static_cast<rAction>(go_rc->action)} {
    source_labels_.reserve(go_rc->source_labels.size());
    for (const auto& sl : go_rc->source_labels) {
      source_labels_.emplace_back(static_cast<std::string_view>(sl));
    }

    static re2::RE2 rgx_validate("(^[\\p{N}\\p{L}_]+)");
    parse(regexp_, rgx_validate, target_label_, target_label_parts_);
    parse(regexp_, rgx_validate, replacement_, replacement_parts_);
  }

  // source_labels - a list of labels from which values are taken and concatenated with the configured separator in order.
  [[nodiscard]] PROMPP_ALWAYS_INLINE const std::vector<std::string_view>& source_labels() const noexcept { return source_labels_; }

  // separator - is the string between concatenated values from the source labels.
  [[nodiscard]] PROMPP_ALWAYS_INLINE const std::string_view& separator() const noexcept { return separator_; }

  // regexp - against which the concatenation is matched.
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Regexp& regexp() const noexcept { return regexp_; }

  // modulus - to take of the hash of concatenated values from the source labels.
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t modulus() const noexcept { return modulus_; }

  // target_label - is the label to which the resulting string is written in a replacement.
  // Regexp interpolation is allowed for the replace action.
  [[nodiscard]] PROMPP_ALWAYS_INLINE const std::string_view& target_label() const noexcept { return target_label_; }

  // replacement - is the regex replacement pattern to be used.
  [[nodiscard]] PROMPP_ALWAYS_INLINE const std::string_view& replacement() const noexcept { return replacement_; }

  // action - is the action to be performed for the relabeling.
  [[nodiscard]] PROMPP_ALWAYS_INLINE rAction action() const noexcept { return action_; }

  // target_label_parts - dismantled target_label.
  [[nodiscard]] PROMPP_ALWAYS_INLINE const std::vector<PatternPart>& target_label_parts() const noexcept { return target_label_parts_; }

  // replacement_parts - dismantled replacement.
  [[nodiscard]] PROMPP_ALWAYS_INLINE const std::vector<PatternPart>& replacement_parts() const noexcept { return replacement_parts_; }

  // relabel - building relabeling labels.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE relabelStatus relabel(std::string& buf, LabelsBuilder& builder) const {
    switch (action_) {
      case rDrop: {
        if (regexp_.full_match(get_value(builder))) {
          return rsDrop;
        }
        break;
      }

      case rKeep: {
        if (!regexp_.full_match(get_value(builder))) {
          return rsDrop;
        }
        break;
      }

      case rDropEqual: {
        if (builder.get(target_label_) == get_value(builder)) {
          return rsDrop;
        }
        break;
      }

      case rKeepEqual: {
        if (builder.get(target_label_) != get_value(builder)) {
          return rsDrop;
        }
        break;
      }

      case rReplace: {
        const auto value = get_value(builder);
        std::vector<std::string_view> res_args;
        if (!regexp_.match_to_args(value, res_args)) {
          break;
        }

        Regexp::replace_with_args(buf, res_args, target_label_parts_);
        if (!label_name_is_valid(buf)) {
          break;
        }
        std::string lname = buf;

        Regexp::replace_with_args(buf, res_args, replacement_parts_);
        if (buf.empty()) {
          if (builder.contains(lname)) {
            builder.del(lname);
            return rsRelabel;
          }
          break;
        }
        builder.set(lname, buf);
        return rsRelabel;
      }

      case rLowercase: {
        auto value = get_value(builder);
        std::ranges::transform(value, value.begin(), [](unsigned char c) { return std::tolower(c); });
        builder.set(target_label_, value);
        return rsRelabel;
      }

      case rUppercase: {
        auto value = get_value(builder);
        std::ranges::transform(value, value.begin(), [](unsigned char c) { return std::toupper(c); });
        builder.set(target_label_, value);
        return rsRelabel;
      }

      case rHashMod: {
        std::string lvalue{std::to_string(make_hash_uint64(get_value(builder)) % modulus_)};
        builder.set(target_label_, lvalue);
        return rsRelabel;
      }

      case rLabelMap: {
        std::vector<Primitives::Label> labels_for_set;
        builder.range([&](const auto& lname, const auto& lvalue) PROMPP_LAMBDA_INLINE -> bool {
          if (regexp_.full_match(lname)) {
            regexp_.replace_full(buf, lname, replacement_parts_);
            labels_for_set.emplace_back(buf, lvalue);
          }
          return true;
        });

        if (!labels_for_set.empty()) {
          for (const auto& label : labels_for_set) {
            builder.set(label.first, label.second);
          }

          return rsRelabel;
        }

        break;
      }

      case rLabelDrop: {
        std::vector<std::string> labels_for_del;
        builder.range([&](const auto& lname, const auto&) PROMPP_LAMBDA_INLINE -> bool {
          if (regexp_.full_match(lname)) {
            labels_for_del.emplace_back(lname);
          }
          return true;
        });

        if (!labels_for_del.empty()) {
          for (const auto& name : labels_for_del) {
            builder.del(name);
          }

          return rsRelabel;
        }

        break;
      }
      case rLabelKeep: {
        std::vector<std::string> labels_for_del;
        builder.range([&](const auto& lname, const auto&) PROMPP_LAMBDA_INLINE -> bool {
          if (!regexp_.full_match(lname)) {
            labels_for_del.emplace_back(lname);
          }
          return true;
        });

        if (!labels_for_del.empty()) {
          for (const auto& name : labels_for_del) {
            builder.del(name);
          }

          return rsRelabel;
        }

        break;
      }

      default: {
        throw BareBones::Exception(0x481dea53751b85c3, "unknown relabel action");
      }
    }

    return rsKeep;
  }
};

// StatelessRelabeler - stateless relabeler with relabel configs.
//
// configs_ - incoming relabel configs;
class StatelessRelabeler {
  std::vector<RelabelConfig> configs_;

 public:
  // StatelessRelabeler - constructor for StatelessRelabeler, converting go-config.
  template <class GORelabelConfigs>
  PROMPP_ALWAYS_INLINE explicit StatelessRelabeler(const GORelabelConfigs& go_rcfgs) noexcept {
    reset_to(go_rcfgs);
  }

  // relabeling_process caller passes a LabelsBuilder containing the initial set of labels, which is mutated by the rules.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE relabelStatus relabeling_process(std::string& buf, LabelsBuilder& builder) const {
    relabelStatus rstatus{rsKeep};
    for (auto& rcfg : configs_) {
      const relabelStatus status = rcfg.relabel(buf, builder);
      if (status == rsDrop) {
        return rsDrop;
      }
      if (status == rsRelabel) {
        rstatus = rsRelabel;
      }
    }

    return rstatus;
  }

  // relabeling_process_with_soft_validate caller passes a LabelsBuilder containing the initial set of labels, which is mutated by the rules with soft(on empty)
  // validate label set.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE relabelStatus relabeling_process_with_soft_validate(std::ostringstream& buf, LabelsBuilder& builder) {
    const relabelStatus rstatus = relabeling_process(buf, builder);
    if (rstatus == rsDrop) {
      return rsDrop;
    }

    if (builder.is_empty()) [[unlikely]] {
      return rsDrop;
    }

    return rstatus;
  }

  // reset_to reset configs and replace on new converting go-config.
  template <class GORelabelConfigs>
  PROMPP_ALWAYS_INLINE void reset_to(const GORelabelConfigs& go_rcfgs) noexcept {
    configs_.clear();
    configs_.reserve(go_rcfgs.size());
    for (const auto go_rcfg : go_rcfgs) {
      configs_.emplace_back(go_rcfg);
    }
  }
};

// processExternalLabels merges externalLabels into ls. If ls contains
// a label in externalLabels, the value in ls wins.
template <class LabelsBuilder, class ExternalLabels>
PROMPP_ALWAYS_INLINE void process_external_labels(LabelsBuilder& builder, const ExternalLabels& external_labels) {
  if (external_labels.size() == 0) {
    return;
  }

  std::size_t j{0};
  std::vector<size_t> indexes_for_set;
  builder.range([&](const auto& lname, const auto&) PROMPP_LAMBDA_INLINE -> bool {
    for (; j < external_labels.size() && lname > external_labels[j].first; ++j) {
      indexes_for_set.emplace_back(j);
    }

    if (j < external_labels.size() && lname == external_labels[j].first) {
      j++;
    }
    return true;
  });

  for (auto index : indexes_for_set) {
    builder.set(external_labels[index].first, external_labels[index].second);
  }

  for (; j < external_labels.size(); j++) {
    builder.set(external_labels[j].first, external_labels[j].second);
  }
}

// soft_validate on empty validate label set.
template <class LabelsBuilder>
PROMPP_ALWAYS_INLINE void soft_validate(relabelStatus& rstatus, LabelsBuilder& builder) {
  if (rstatus == rsDrop) {
    return;
  }

  if (builder.is_empty()) [[unlikely]] {
    rstatus = rsDrop;
  }
};

}  // namespace PromPP::Prometheus::Relabel
