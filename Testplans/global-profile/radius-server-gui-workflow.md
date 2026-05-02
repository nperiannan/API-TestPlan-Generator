# RADIUS Servers — GUI Workflow Test Cases

**Module:** Network Configuration  
**Feature:** RADIUS Servers (Global Profiles)  
**Navigation:** Configuration → Third-Party Management → Network Configuration → Global Profiles → Feature Navigator → Radius Servers  
**Source:** UI screenshots (5 screens analyzed)

---

## A. Page Load / Empty State

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_001 | Verify RADIUS Servers page loads with correct layout | User is logged in with admin role | 1. Click "Configuration" in left nav<br>2. Click "Third-Party Management"<br>3. Click "Global Profiles" tab<br>4. In Feature Navigator, click "Radius Servers" | Observe the page layout | - Page title shows "RADIUS Servers" with "View Usage" link<br>- "Platform ONE Security" toggle is visible (default OFF)<br>- Search bar is present<br>- "+ Add RADIUS Server" button is visible<br>- Refresh and Export icons are present<br>- Table columns: Priority, IP Address/FQDN, Enabled/Disabled, Type, RADSec, Name, Authentication Port<br>- "Columns" and "Filters" sidebars available on right edge |
| GUI_RS_002 | Verify empty state message when no servers configured | No RADIUS servers exist in the system | Navigate to RADIUS Servers page | Observe the table area | - Table body shows empty state icon<br>- Message: "No Radius Server Configuration Found." |
| GUI_RS_003 | Verify Feature Navigator highlights active feature | User is on Network Configuration page | Click "Radius Servers" in Feature Navigator | Observe Feature Navigator on right side | - "Radius Servers" is highlighted/selected (purple text)<br>- Other features (NTP Servers, DNS, DHCP Servers, Syslog Servers) are not highlighted<br>- "Global Features" section is expanded |
| GUI_RS_004 | Verify "View Usage" link is present and clickable | User is on RADIUS Servers page | Click "View Usage" next to page title | Observe behavior | - "View Usage" link is visible next to "RADIUS Servers" title<br>- **Assumption/Needs confirmation:** Opens a view showing where this RADIUS config is used |
| GUI_RS_005 | Verify search bar is present on empty table | No RADIUS servers exist | Navigate to RADIUS Servers page | Click on the Search bar | - Search input is active and accepts text<br>- **Assumption/Needs confirmation:** Search filters the table rows in real-time or on enter |
| GUI_RS_006 | Verify Columns sidebar toggle | User is on RADIUS Servers page | Click "Columns" toggle on right edge | Observe sidebar | - **Assumption/Needs confirmation:** A sidebar opens allowing show/hide of table columns |
| GUI_RS_007 | Verify Filters sidebar toggle | User is on RADIUS Servers page | Click "Filters" toggle on right edge | Observe sidebar | - **Assumption/Needs confirmation:** A sidebar opens with filter options for the table |

---

## B. Platform ONE Security Toggle

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_010 | Verify Platform ONE Security toggle default state | User is on RADIUS Servers page, no prior changes | Navigate to RADIUS Servers page | Observe "Platform ONE Security" toggle | - Toggle is in OFF position (grey)<br>- Label reads "Platform ONE Security" |
| GUI_RS_011 | Verify enabling Platform ONE Security shows confirmation dialog | Platform ONE Security toggle is OFF | Navigate to RADIUS Servers page | Click the "Platform ONE Security" toggle to ON | - Confirmation dialog appears: "Enable Platform ONE Security?"<br>- Dialog text: "Are you sure you want to enable Platform ONE Security? It will be synced to Configuration Profiles inheriting the global RADIUS Servers."<br>- Additional note: "Deployment of this Configuration Profile is required for the changes to take effect on the assigned devices."<br>- "View Inheriting Configuration Profiles" expandable section is visible<br>- Buttons: "Cancel" and "Enable" |
| GUI_RS_012 | Verify inheriting profiles section in enable dialog | Platform ONE Security toggle is OFF | Click toggle to ON | In the confirmation dialog, expand "View Inheriting Configuration Profiles" | - Expandable section opens<br>- Shows a Search bar<br>- Table with columns: Configuration Profile, Deployment Status<br>- If no profiles: "No configuration profiles found" |
| GUI_RS_013 | Verify confirming "Enable" activates Platform ONE Security | Confirmation dialog is open | Click "Enable" button | Observe the toggle and page state | - Dialog closes<br>- Toggle moves to ON position (purple/blue)<br>- **Assumption/Needs confirmation:** RADIUS server type becomes "Platform ONE Security" for applicable servers |
| GUI_RS_014 | Verify canceling the enable dialog keeps toggle OFF | Confirmation dialog is open | Click "Cancel" button | Observe the toggle | - Dialog closes<br>- Toggle remains in OFF position (grey)<br>- No changes applied |
| GUI_RS_015 | Verify closing enable dialog with X keeps toggle OFF | Confirmation dialog is open | Click "X" close button on dialog | Observe the toggle | - Dialog closes<br>- Toggle remains in OFF position |
| GUI_RS_016 | Verify disabling Platform ONE Security (toggle OFF) | Platform ONE Security toggle is ON | Click the toggle to OFF | Observe behavior | - **Assumption/Needs confirmation:** A confirmation dialog may appear asking to disable<br>- Toggle moves to OFF (grey) |

---

## C. Add RADIUS Server — Modal Behavior

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_020 | Verify clicking "Add RADIUS Server" opens modal | User is on RADIUS Servers page | Click "+ Add RADIUS Server" button | Observe modal | - Modal opens with title "Add RADIUS Server"<br>- "Advanced" toggle at top-right (default OFF)<br>- X close button visible<br>- Left sidebar shows two tabs: "RADIUS Server Details" (active), "Inheriting Configuration Profiles" (Optional)<br>- Form fields visible in "RADIUS Server Details" section |
| GUI_RS_021 | Verify modal has two tabs | Add RADIUS Server modal is open | Observe the left sidebar tabs | - Tab 1: "RADIUS Server Details" (active by default, highlighted)<br>- Tab 2: "Inheriting Configuration Profiles" with "Optional" subtitle |
| GUI_RS_022 | Verify switching to "Inheriting Configuration Profiles" tab | Add RADIUS Server modal is open | Click "Inheriting Configuration Profiles" tab | - Tab becomes active (highlighted with blue border)<br>- Content area shows "Inheriting Configuration Profiles" title<br>- Search bar present<br>- Table with columns: Configuration Profile, Deployment Status<br>- Empty state: "No configuration profiles found"<br>- Scrollbar visible at bottom |
| GUI_RS_023 | Verify switching back to "RADIUS Server Details" tab | "Inheriting Configuration Profiles" tab is active | Click "RADIUS Server Details" tab | - Tab becomes active<br>- Form fields for RADIUS Server Details are displayed again<br>- Previously entered values (if any) are preserved |
| GUI_RS_024 | Verify closing modal with X button | Add RADIUS Server modal is open | Click X button at top-right | - Modal closes<br>- Returns to RADIUS Servers list page<br>- **Assumption/Needs confirmation:** Unsaved data is discarded (or a discard confirmation appears) |
| GUI_RS_025 | Verify modal overlay blocks background interaction | Add RADIUS Server modal is open | Try clicking on the background page elements | - Background is dimmed/overlaid<br>- Clicks on background do not interact with the main page |

---

## D. Add RADIUS Server — Form Fields (Standard Mode)

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_030 | Verify all standard form fields are present | Add RADIUS Server modal is open, Advanced OFF | Observe the form | - "IP Address/FQDN" text input with helper text "Supports IPv4, and IPv6."<br>- "Name (Optional)" text input<br>- "Authentication" toggle (default OFF)<br>- "Accounting" toggle (default OFF)<br>- "Authentication Port" text input<br>- "Shared Secret" password input (under Authentication) with eye toggle icon<br>- "Accounting Port" text input<br>- "Shared Secret" password input (under Accounting) with eye toggle icon |
| GUI_RS_031 | Verify IP Address/FQDN field accepts valid IPv4 | Modal open | Enter "10.0.0.1" in IP Address/FQDN | - Field accepts the value<br>- No validation error shown |
| GUI_RS_032 | Verify IP Address/FQDN field accepts valid IPv6 | Modal open | Enter "2001:db8::1" in IP Address/FQDN | - Field accepts the value<br>- No validation error shown |
| GUI_RS_033 | Verify IP Address/FQDN field accepts FQDN | Modal open | Enter "radius.example.com" in IP Address/FQDN | - Field accepts the value<br>- No validation error shown |
| GUI_RS_034 | Verify Name field is optional | Modal open | Leave "Name (Optional)" empty and fill all required fields, click Save | - Server is created successfully without a name<br>- **Assumption/Needs confirmation:** Name defaults to empty or auto-generated |
| GUI_RS_035 | Verify Name field accepts valid input | Modal open | Enter "Radius-Server-1" in Name field | - Field accepts the value<br>- No validation error |
| GUI_RS_036 | Verify Shared Secret fields have masked input | Modal open | Enter text in either "Shared Secret" field | - Characters are masked (shown as dots/bullets)<br>- Eye toggle icon is visible to show/hide the value |
| GUI_RS_037 | Verify Shared Secret eye toggle reveals password | Modal open, text entered in Shared Secret | Click the eye icon on Shared Secret field | - Password text becomes visible in plain text<br>- Eye icon toggles to "hide" state |
| GUI_RS_038 | Verify Authentication Port default value | Modal open, Authentication toggle is ON | Observe Authentication Port field | - **Assumption/Needs confirmation:** Field may show default value 1812 or be empty for user input |
| GUI_RS_039 | Verify Accounting Port default value | Modal open, Accounting toggle is ON | Observe Accounting Port field | - **Assumption/Needs confirmation:** Field may show default value 1813 or be empty for user input |

---

## E. Toggle Behavior (Authentication / Accounting)

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_040 | Verify Authentication toggle default state is OFF | Add RADIUS Server modal is open | Observe Authentication toggle | - Toggle is OFF (grey)<br>- Authentication Port and Shared Secret fields appear disabled/greyed out |
| GUI_RS_041 | Verify Accounting toggle default state is OFF | Add RADIUS Server modal is open | Observe Accounting toggle | - Toggle is OFF (grey)<br>- Accounting Port and Shared Secret fields appear disabled/greyed out |
| GUI_RS_042 | Verify enabling Authentication toggle enables fields | Modal open, Authentication toggle OFF | Click Authentication toggle to ON | - Toggle changes to ON (purple/blue)<br>- "Authentication Port" field becomes enabled/editable<br>- "Shared Secret" field (under Authentication) becomes enabled/editable |
| GUI_RS_043 | Verify enabling Accounting toggle enables fields | Modal open, Accounting toggle OFF | Click Accounting toggle to ON | - Toggle changes to ON (purple/blue)<br>- "Accounting Port" field becomes enabled/editable<br>- "Shared Secret" field (under Accounting) becomes enabled/editable |
| GUI_RS_044 | Verify disabling Authentication toggle after filling fields | Modal open, Authentication ON with values entered | Turn Authentication toggle OFF | - Toggle changes to OFF (grey)<br>- Authentication Port and Shared Secret fields become disabled/greyed out<br>- **Assumption/Needs confirmation:** Previously entered values may be cleared or retained but disabled |
| GUI_RS_045 | Verify disabling Accounting toggle after filling fields | Modal open, Accounting ON with values entered | Turn Accounting toggle OFF | - Toggle changes to OFF (grey)<br>- Accounting Port and Shared Secret fields become disabled/greyed out<br>- **Assumption/Needs confirmation:** Previously entered values may be cleared or retained but disabled |
| GUI_RS_046 | Verify both toggles can be ON simultaneously | Modal open | Turn both Authentication and Accounting toggles ON | - Both toggles are ON (purple/blue)<br>- All four sub-fields (Auth Port, Auth Secret, Acct Port, Acct Secret) are enabled |
| GUI_RS_047 | Verify saving with both toggles OFF | Modal open, both toggles OFF | Fill IP Address/FQDN, click Save | - **Assumption/Needs confirmation:** Server is created with auth-server-enabled=false and accounting-server-enabled=false |

---

## F. Advanced Mode Toggle

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_050 | Verify Advanced toggle default state is OFF | Add RADIUS Server modal is open | Observe "Advanced" toggle at top-right | - Toggle is OFF (grey)<br>- Only standard fields are visible (IP, Name, Auth toggle, Acct toggle, ports, secrets) |
| GUI_RS_051 | Verify enabling Advanced mode shows additional fields | Modal open, Advanced OFF | Click "Advanced" toggle to ON | - Toggle changes to ON (purple/blue)<br>- Additional fields appear at bottom of form:<br>  • "Organization (Optional)" text input<br>  • "Message Authenticator" dropdown (with chevron) |
| GUI_RS_052 | Verify Organization field is optional in Advanced mode | Advanced ON | Leave "Organization (Optional)" empty, fill required fields, click Save | - Server is created successfully without Organization value |
| GUI_RS_053 | Verify Message Authenticator dropdown options | Advanced ON | Click "Message Authenticator" dropdown | - Dropdown opens with selectable options<br>- **Assumption/Needs confirmation:** Options may include "REQUIRED" and "NOT_REQUIRED" based on YANG model |
| GUI_RS_054 | Verify disabling Advanced mode hides extra fields | Advanced ON with values entered in Organization and Message Authenticator | Click "Advanced" toggle to OFF | - Extra fields (Organization, Message Authenticator) are hidden<br>- **Assumption/Needs confirmation:** Values may be cleared or retained internally |
| GUI_RS_055 | Verify re-enabling Advanced mode retains values | Advanced was ON with values, toggled OFF then ON again | Toggle Advanced OFF then ON | - Organization and Message Authenticator fields reappear<br>- **Assumption/Needs confirmation:** Previously entered values may be retained or cleared |

---

## G. Field Validation (Negative Scenarios)

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_060 | Verify IP Address/FQDN is required | Modal open | Leave IP Address/FQDN empty, click Save | - Validation error appears on IP Address/FQDN field<br>- Save is blocked<br>- Error message indicates field is required |
| GUI_RS_061 | Verify invalid IP address is rejected | Modal open | Enter "999.999.999.999" in IP Address/FQDN, click Save | - Validation error on the field<br>- **Assumption/Needs confirmation:** Error message like "Invalid IP address" |
| GUI_RS_062 | Verify invalid IPv6 is rejected | Modal open | Enter "not:a:valid:ipv6" in IP Address/FQDN | - Validation error displayed |
| GUI_RS_063 | Verify special characters in IP field | Modal open | Enter "!@#$%^&*()" in IP Address/FQDN | - Validation error or input rejected |
| GUI_RS_064 | Verify empty string in IP field | Modal open | Enter spaces only in IP Address/FQDN, click Save | - Validation error — field treated as empty |
| GUI_RS_065 | Verify Authentication Port accepts valid port | Modal open, Authentication ON | Enter "1812" in Authentication Port | - Field accepts value, no error |
| GUI_RS_066 | Verify Authentication Port rejects non-numeric input | Modal open, Authentication ON | Enter "abc" in Authentication Port | - Field rejects non-numeric input or shows validation error |
| GUI_RS_067 | Verify Authentication Port rejects out-of-range port | Modal open, Authentication ON | Enter "99999" in Authentication Port | - Validation error for port out of valid range (1–65535) |
| GUI_RS_068 | Verify Authentication Port rejects negative number | Modal open, Authentication ON | Enter "-1" in Authentication Port | - Validation error or input rejected |
| GUI_RS_069 | Verify Accounting Port rejects non-numeric input | Modal open, Accounting ON | Enter "xyz" in Accounting Port | - Field rejects non-numeric input or shows validation error |
| GUI_RS_070 | Verify Shared Secret is required when Authentication is ON | Modal open, Authentication ON | Leave Auth Shared Secret empty, click Save | - **Assumption/Needs confirmation:** Validation error on Shared Secret field — required when auth is enabled |
| GUI_RS_071 | Verify Shared Secret is required when Accounting is ON | Modal open, Accounting ON | Leave Acct Shared Secret empty, click Save | - **Assumption/Needs confirmation:** Validation error on Shared Secret field — required when accounting is enabled |
| GUI_RS_072 | Verify duplicate IP Address/FQDN is rejected | One RADIUS server already exists with IP "10.0.0.1" | Open Add modal, enter "10.0.0.1" in IP Address/FQDN, click Save | - **Assumption/Needs confirmation:** Error indicating duplicate server IP |
| GUI_RS_073 | Verify max length on Name field | Modal open | Enter a very long string (e.g., 256+ characters) in Name | - **Assumption/Needs confirmation:** Input truncated or validation error at max length |

---

## H. Save / Cancel / Save & Add Another Flows

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_080 | Verify successful save with minimum required fields | Modal open | 1. Enter "10.0.0.1" in IP Address/FQDN<br>2. Click "Save" | - Modal closes<br>- Success notification/toast appears<br>- New RADIUS server appears in the table<br>- Server row shows: Priority=auto, IP Address/FQDN=10.0.0.1, Enabled/Disabled value |
| GUI_RS_081 | Verify successful save with all standard fields | Modal open | 1. Enter "10.0.0.1" in IP Address/FQDN<br>2. Enter "Radius-Server-1" in Name<br>3. Turn Authentication ON<br>4. Enter "1812" in Authentication Port<br>5. Enter "secret123" in Auth Shared Secret<br>6. Turn Accounting ON<br>7. Enter "1813" in Accounting Port<br>8. Enter "secret456" in Acct Shared Secret<br>9. Click "Save" | - Modal closes<br>- Success notification appears<br>- Server in table shows all values correctly:<br>  Priority, IP=10.0.0.1, Enabled/Disabled=Enabled, Name=Radius-Server-1, Authentication Port=1812 |
| GUI_RS_082 | Verify "Save & Add Another" keeps modal open | Modal open with valid data entered | Click "Save & Add Another" | - Current server is saved (success notification)<br>- Modal stays open<br>- Form fields are cleared/reset for next entry<br>- Toggles reset to OFF, Advanced reset to OFF |
| GUI_RS_083 | Verify "Save & Add Another" then save second server | Modal open after first save via "Save & Add Another" | 1. Enter "10.0.0.2" in IP Address/FQDN<br>2. Click "Save" | - Second server saved<br>- Modal closes<br>- Table shows both servers |
| GUI_RS_084 | Verify Cancel button discards unsaved data | Modal open with data entered | Click "Cancel" | - Modal closes<br>- No new server is added to the table<br>- No success/error notification |
| GUI_RS_085 | Verify Cancel with no data entered | Modal open, no fields filled | Click "Cancel" | - Modal closes cleanly<br>- No error or confirmation dialog |

---

## I. Table Behavior (After Server Created)

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_090 | Verify table shows newly added server | At least one RADIUS server is configured | Navigate to RADIUS Servers page | Observe the table | - Table shows server rows with data in all columns:<br>  Priority, IP Address/FQDN, Enabled/Disabled, Type, RADSec, Name, Authentication Port<br>- Empty state message no longer shown |
| GUI_RS_091 | Verify table column values match saved data | Server "10.0.0.1" / "Radius-Server-1" exists | Observe table row | - Priority shows assigned value (e.g., 1)<br>- IP Address/FQDN = "10.0.0.1"<br>- Name = "Radius-Server-1"<br>- Authentication Port = 1812 |
| GUI_RS_092 | Verify search filters table results | Multiple servers exist | Enter a server name or IP in the Search bar | - Table filters to show only matching rows<br>- Non-matching rows are hidden |
| GUI_RS_093 | Verify search with no results | Servers exist but none match query | Enter "nonexistent" in Search bar | - Table shows no rows<br>- **Assumption/Needs confirmation:** Empty state or "No results" message shown |
| GUI_RS_094 | Verify refresh button reloads table data | Servers exist in table | Click the Refresh icon (circular arrow) | - Table data is refreshed/reloaded from server<br>- Any recent changes from other users are reflected |
| GUI_RS_095 | Verify export/download button | Servers exist in table | Click the Export/Download icon | - **Assumption/Needs confirmation:** Downloads server list as CSV/Excel or opens export options |
| GUI_RS_096 | Verify table row checkbox selection | Servers exist in table | Click the checkbox on a table row | - Row is selected/highlighted<br>- **Assumption/Needs confirmation:** Bulk action options may appear (e.g., Delete selected) |
| GUI_RS_097 | Verify select all checkbox | Multiple servers in table | Click the header checkbox (next to "Priority") | - All rows are selected<br>- **Assumption/Needs confirmation:** Bulk action toolbar appears |
| GUI_RS_098 | Verify table row click opens edit/detail view | Server exists | Click on a server row (not checkbox) | - **Assumption/Needs confirmation:** Edit modal opens or detail panel slides in with server details pre-filled |

---

## J. Edit / Delete Workflows

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_100 | Verify editing an existing RADIUS server | Server "10.0.0.1" exists | Click on the server row to open edit | - Edit modal opens with pre-filled values<br>- IP Address/FQDN = "10.0.0.1"<br>- Name = previously saved name<br>- Toggle states match saved configuration<br>- **Assumption/Needs confirmation:** IP/FQDN field may be read-only (key field) |
| GUI_RS_101 | Verify modifying server name and saving | Edit modal open for existing server | 1. Change Name to "Updated-Server"<br>2. Click Save | - Modal closes<br>- Success notification<br>- Table row shows updated Name = "Updated-Server" |
| GUI_RS_102 | Verify enabling Authentication on existing server | Edit modal open, Authentication OFF | 1. Turn Authentication toggle ON<br>2. Enter "1812" in Auth Port<br>3. Enter "newsecret" in Auth Shared Secret<br>4. Click Save | - Server updated successfully<br>- Enabled/Disabled column reflects new state |
| GUI_RS_103 | Verify deleting a RADIUS server | Server exists, row selected | 1. Select server row<br>2. Click Delete action | - **Assumption/Needs confirmation:** Confirmation dialog appears<br>- After confirming, server is removed from table<br>- If last server, empty state message reappears |
| GUI_RS_104 | Verify canceling delete operation | Delete confirmation dialog open | Click "Cancel" on delete dialog | - Dialog closes<br>- Server remains in table |

---

## K. Full End-to-End Workflow

| Test Case ID | Title | Preconditions | Navigation Steps | Test Steps | Expected Behavior |
|---|---|---|---|---|---|
| GUI_RS_110 | Complete CRUD workflow for RADIUS Server | User logged in as admin, no existing servers | 1. Navigate to Configuration → Third-Party Management<br>2. Click Global Profiles tab<br>3. Click Radius Servers in Feature Navigator | **Create:**<br>1. Click "+ Add RADIUS Server"<br>2. Enter "10.0.0.1" in IP Address/FQDN<br>3. Enter "Radius-Server-1" in Name<br>4. Turn Authentication ON<br>5. Enter "1812" in Authentication Port<br>6. Enter "secret123" in Shared Secret<br>7. Turn Accounting ON<br>8. Enter "1813" in Accounting Port<br>9. Enter "secret456" in Shared Secret<br>10. Click Save<br><br>**Read:**<br>11. Verify server row in table with correct values<br><br>**Update:**<br>12. Click server row to edit<br>13. Change Name to "Updated-Server-1"<br>14. Click Save<br>15. Verify updated name in table<br><br>**Delete:**<br>16. Select server row<br>17. Delete server<br>18. Confirm deletion<br>19. Verify empty state returns | - Create: Modal closes, toast "success", row appears with correct data<br>- Read: All columns populated correctly<br>- Update: Name updates in table<br>- Delete: Server removed, "No Radius Server Configuration Found." shown |
| GUI_RS_111 | Platform ONE Security enable with server creation | User logged in, toggle is OFF | Navigate to RADIUS Servers page | 1. Click "Platform ONE Security" toggle<br>2. In confirmation dialog, click "Enable"<br>3. Verify toggle is ON<br>4. Click "+ Add RADIUS Server"<br>5. Toggle Advanced ON<br>6. Fill all fields including Organization and Message Authenticator<br>7. Click "Save"<br>8. Verify table row shows Type = "Platform ONE Security" | - Toggle ON after confirmation<br>- Server created with all advanced fields<br>- Table reflects correct Type value |

---

## Field-to-API Property Mapping Reference

| UI Field Label | API Property Name | Type | Required | Section |
|---|---|---|---|---|
| IP Address/FQDN | `server` | string | Yes | Standard |
| Name (Optional) | `name` | string | No | Standard |
| Authentication toggle | `auth-server-enabled` | boolean | No (default false) | Standard |
| Accounting toggle | `accounting-server-enabled` | boolean | No (default false) | Standard |
| Authentication Port | `auth-server-port` | number | When auth ON | Standard |
| Shared Secret (Auth) | `auth-server-secret` | string | When auth ON | Standard |
| Accounting Port | `accounting-server-port` | number | When acct ON | Standard |
| Shared Secret (Acct) | `accounting-server-secret` | string | When acct ON | Standard |
| Organization (Optional) | `operator-name` | string | No | Advanced |
| Message Authenticator | `message-authenticator` | enumeration | No | Advanced |
| Priority | `priority` | number | Auto-assigned | Table column |
| Enabled/Disabled | derived from toggles | display | — | Table column |
| Type | `radius-server-type` | enumeration | — | Table column |
| RADSec | derived from secure-mode | display | — | Table column |

**Note:** Fields `id`, `created-at`, `customer-id`, `owner-id`, `deleted-at`, `vr-name`, `auth-secret-is-encrypted`, `accounting-secret-is-encrypted`, `auth-secure-mode`, `accounting-secure-mode` are system-managed and NOT visible as user inputs in the UI.
