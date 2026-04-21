"""
resolve_all_schemas.py
Iteratively finds all missing $ref schemas and adds them from PlatformServices
until no missing schemas remain.
"""
import re

ENRICHED = r"C:\Users\nperiannan\OneDrive - Extreme Networks, Inc\Desktop\Testcase generator\qaopenapi.yaml"
PLATFORM = r"C:\Natarajan\automation\PlatformServices\Configuration\src\configuration\infra\rest\openapi.yaml"

with open(PLATFORM, encoding='utf-8', errors='replace') as f:
    platform_lines = f.readlines()

# Pre-build full platform schema map once
print("Building platform schema index...")
platform_schema_blocks = {}
i = 0
while i < len(platform_lines):
    m = re.match(r'^    (\w+):\s*$', platform_lines[i])
    if m:
        name = m.group(1)
        start = i
        i += 1
        while i < len(platform_lines):
            if re.match(r'^    \w+:\s*$', platform_lines[i]) or re.match(r'^  \w+:', platform_lines[i]):
                break
            i += 1
        platform_schema_blocks[name] = platform_lines[start:i]
    else:
        i += 1
print("Platform has " + str(len(platform_schema_blocks)) + " schemas")

round_num = 0
total_added = 0

while True:
    round_num += 1
    with open(ENRICHED, encoding='utf-8', errors='replace') as f:
        enriched_lines = f.readlines()
    enriched_text = "".join(enriched_lines)

    defined = set(re.findall(r'^    (\w+):\s*$', enriched_text, re.MULTILINE))
    refs = set(re.findall(r'#/components/schemas/(\w+)', enriched_text))
    missing = sorted(refs - defined)

    if not missing:
        print("\nAll schemas resolved after " + str(round_num-1) + " rounds! Total added: " + str(total_added))
        break

    in_platform = [s for s in missing if s in platform_schema_blocks]
    not_in_platform = [s for s in missing if s not in platform_schema_blocks]

    if not_in_platform:
        print("Round " + str(round_num) + ": " + str(len(not_in_platform)) + " schemas not in PlatformServices (will skip): " + str(not_in_platform))

    if not in_platform:
        print("Round " + str(round_num) + ": No more resolvable schemas. Remaining missing: " + str(not_in_platform))
        break

    print("Round " + str(round_num) + ": Adding " + str(len(in_platform)) + " schemas: " + str(in_platform))

    # Find securitySchemes: insertion point
    sec_line = None
    for i, l in enumerate(enriched_lines):
        if re.match(r'^  securitySchemes:', l):
            sec_line = i
            break
    if sec_line is None:
        sec_line = len(enriched_lines)  # append at end

    insert_lines = []
    for name in in_platform:
        insert_lines.extend(platform_schema_blocks[name])
        if insert_lines and insert_lines[-1].strip():
            insert_lines.append("\n")

    final = enriched_lines[:sec_line] + insert_lines + enriched_lines[sec_line:]
    with open(ENRICHED, 'w', encoding='utf-8') as f:
        f.writelines(final)

    total_added += len(in_platform)
    print("  File now has " + str(len(final)) + " lines")

# Final check
with open(ENRICHED, encoding='utf-8', errors='replace') as f:
    enriched_text = f.read()
defined = set(re.findall(r'^    (\w+):\s*$', enriched_text, re.MULTILINE))
refs = set(re.findall(r'#/components/schemas/(\w+)', enriched_text))
still_missing = refs - defined
if still_missing:
    print("Still missing (not in PlatformServices either): " + str(sorted(still_missing)))
else:
    print("SUCCESS: All " + str(len(refs)) + " schema refs are defined.")
