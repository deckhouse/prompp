#pragma once

#include "bare_bones/snug_composite_v2.h"
#include "snug_composites_filaments.h"

namespace PromPP::Primitives::SnugComposites {

namespace Symbol {

template <template <class> class Vector>
using DecodingTable = BareBones::SnugComposite::V2::DecodingTable<Filaments::Symbol, Vector>;

template <template <class> class Vector>
using EncodingBimap = BareBones::SnugComposite::V2::EncodingBimap<Filaments::Symbol, Vector>;

}  // namespace Symbol

namespace LabelNameSet {

template <template <class> class Vector>
using DecodingTableFilament = Filaments::LabelNameSet<Symbol::DecodingTable, Vector>;

template <template <class> class Vector>
using DecodingTable = BareBones::SnugComposite::V2::DecodingTable<DecodingTableFilament, Vector>;

template <template <class> class Vector>
using EncodingBimapFilament = Filaments::LabelNameSet<Symbol::EncodingBimap, Vector>;

template <template <class> class Vector>
using EncodingBimap = BareBones::SnugComposite::V2::EncodingBimap<EncodingBimapFilament, Vector>;

}  // namespace LabelNameSet

namespace LabelSet {

template <template <class> class Vector>
using DecodingTableFilament = Filaments::LabelSet<Symbol::DecodingTable, LabelNameSet::DecodingTable, Vector>;

template <template <class> class Vector>
using DecodingTable = BareBones::SnugComposite::V2::DecodingTable<DecodingTableFilament, Vector>;

template <template <class> class Vector>
using EncodingBimapFilament = Filaments::LabelSet<Symbol::EncodingBimap, LabelNameSet::EncodingBimap, Vector>;

template <template <class> class Vector>
using EncodingBimap = BareBones::SnugComposite::V2::EncodingBimap<EncodingBimapFilament, Vector>;

template <template <class> class Vector>
using ShrinkableEncodingBimap = BareBones::SnugComposite::ShrinkableEncodingBimap<EncodingBimapFilament, Vector>;

}  // namespace LabelSet

}  // namespace PromPP::Primitives::SnugComposites