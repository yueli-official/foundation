# Search Context

Search provides a common language for deriving searchable candidates from product content without becoming the content or authorization truth.

## Language

**Source Document**:
A product-owned content object from which searchable information is derived.
_Avoid_: Index record, search row

**Search Document**:
The minimal, rebuildable representation of a Source Document used to find candidates.
_Avoid_: Content copy, source record

**Projection Revision**:
A monotonically increasing version of one Source Document's searchable representation.
_Avoid_: Updated time, index timestamp

**Index**:
The current searchable collection of Search Documents for one independent product instance.
_Avoid_: Search service, content database

**Query**:
A reader's text intent together with declared filters, facets, ordering and page request.
_Avoid_: SQL query, backend DSL

**Search Plan**:
The normalized interpretation of a Query under one Definition, Adapter and Rebuild Generation.
_Avoid_: Query, SQL plan

**Hit**:
An Index candidate that references a Source Document and still requires current product visibility validation.
_Avoid_: Authorized result, content result

**Visibility Reference**:
An opaque product resource reference used to recheck whether the current subject may receive a Hit.
_Avoid_: Search permission, role filter

**Rebuild Generation**:
An isolated version of the Index that can be populated and atomically made active.
_Avoid_: Index version, deployment
