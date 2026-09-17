# Project Structure

Every file tracked in the repository. Regenerate with
`scripts/filelist_update.sh` after adding or moving files.

    .
    ├── .agents
    │   ├── agents
    │   │   ├── best-practices-sidecar.md
    │   │   ├── commit-preparer.md
    │   │   ├── docs-auditor.md
    │   │   ├── implement-coordinator.md
    │   │   ├── implement-worker.md
    │   │   ├── loop-critic.md
    │   │   ├── loop-evaluator.md
    │   │   ├── loop-invariant-prep.md
    │   │   ├── loop-orchestrator.md
    │   │   ├── loop-perf-prep.md
    │   │   ├── loop-planner.md
    │   │   ├── loop-producer.md
    │   │   ├── loop-refiner.md
    │   │   ├── loop-test-prep.md
    │   │   ├── plan-coordinator.md
    │   │   ├── plan-polisher.md
    │   │   ├── review-sidecar.md
    │   │   ├── rules-sidecar.md
    │   │   └── security-sidecar.md
    │   └── skills
    │       ├── aif
    │       │   ├── references
    │       │   │   ├── config-template.yaml
    │       │   │   └── update-config.mjs
    │       │   └── SKILL.md
    │       ├── aif-architecture
    │       │   ├── references
    │       │   │   └── architecture.md
    │       │   └── SKILL.md
    │       ├── aif-archive
    │       │   └── SKILL.md
    │       ├── aif-best-practices
    │       │   └── SKILL.md
    │       ├── aif-build-automation
    │       │   ├── references
    │       │   │   ├── BEST-PRACTICES.md
    │       │   │   ├── DOC-INTEGRATION.md
    │       │   │   └── SUMMARY-FORMAT.md
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       ├── justfile-go
    │       │       ├── justfile-gradle
    │       │       ├── justfile-maven
    │       │       ├── justfile-node
    │       │       ├── justfile-php
    │       │       ├── justfile-python
    │       │       ├── justfile-ruby
    │       │       ├── justfile-rust
    │       │       ├── magefile-basic.go
    │       │       ├── magefile-full.go
    │       │       ├── makefile-go.mk
    │       │       ├── makefile-gradle.mk
    │       │       ├── makefile-maven.mk
    │       │       ├── makefile-node.mk
    │       │       ├── makefile-php.mk
    │       │       ├── makefile-python.mk
    │       │       ├── makefile-ruby.mk
    │       │       ├── makefile-rust.mk
    │       │       ├── taskfile-go.yml
    │       │       ├── taskfile-gradle.yml
    │       │       ├── taskfile-maven.yml
    │       │       ├── taskfile-node.yml
    │       │       ├── taskfile-php.yml
    │       │       ├── taskfile-python.yml
    │       │       ├── taskfile-ruby.yml
    │       │       └── taskfile-rust.yml
    │       ├── aif-ci
    │       │   ├── references
    │       │   │   ├── AUDIT-REPORT.md
    │       │   │   ├── BEST-PRACTICES.md
    │       │   │   ├── GITLAB-PATTERNS.md
    │       │   │   ├── SERVICE-CONTAINERS.md
    │       │   │   └── TOOL-COMMANDS.md
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       ├── github
    │       │       │   ├── go.yml
    │       │       │   ├── java.yml
    │       │       │   ├── node.yml
    │       │       │   ├── php.yml
    │       │       │   ├── python.yml
    │       │       │   └── rust.yml
    │       │       └── gitlab
    │       │           ├── go.yml
    │       │           ├── java.yml
    │       │           ├── node.yml
    │       │           ├── php.yml
    │       │           ├── python.yml
    │       │           └── rust.yml
    │       ├── aif-commit
    │       │   └── SKILL.md
    │       ├── aif-docs
    │       │   ├── references
    │       │   │   └── REVIEW-CHECKLISTS.md
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       └── html-template.html
    │       ├── aif-evolve
    │       │   └── SKILL.md
    │       ├── aif-explore
    │       │   ├── references
    │       │   │   └── ULTRA-RESEARCH-FORMAT.md
    │       │   └── SKILL.md
    │       ├── aif-fix
    │       │   └── SKILL.md
    │       ├── aif-grounded
    │       │   └── SKILL.md
    │       ├── aif-implement
    │       │   ├── references
    │       │   │   ├── IMPLEMENTATION-GUIDE.md
    │       │   │   └── LOGGING-GUIDE.md
    │       │   └── SKILL.md
    │       ├── aif-improve
    │       │   ├── references
    │       │   │   ├── CHECK-MODE.md
    │       │   │   ├── EXAMPLES.md
    │       │   │   ├── LIST-MODE.md
    │       │   │   └── VALIDATOR.md
    │       │   └── SKILL.md
    │       ├── aif-loop
    │       │   ├── references
    │       │   │   ├── ACTIVE-TIME-BUDGET.md
    │       │   │   ├── CONTEXT-MANAGEMENT.md
    │       │   │   ├── CRITERIA-TEMPLATES.md
    │       │   │   ├── PHASE-CONTRACTS.md
    │       │   │   ├── RULE-SCHEMA.md
    │       │   │   └── TERMINAL-REPORT.md
    │       │   └── SKILL.md
    │       ├── aif-plan
    │       │   ├── references
    │       │   │   ├── EXAMPLES.md
    │       │   │   ├── TASK-FORMAT.md
    │       │   │   └── ULTRA-FORMAT.md
    │       │   └── SKILL.md
    │       ├── aif-qa
    │       │   ├── references
    │       │   │   ├── CHANGE-SUMMARY.md
    │       │   │   ├── TEST-CASES.md
    │       │   │   └── TEST-PLAN.md
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       ├── CHANGE-SUMMARY.md
    │       │       ├── TEST-CASES.md
    │       │       └── TEST-PLAN.md
    │       ├── aif-qa-check
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       └── QA-CHECK.md
    │       ├── aif-review
    │       │   ├── references
    │       │   │   ├── CHECK-MODE.md
    │       │   │   ├── SEVERITY.md
    │       │   │   └── VALIDATOR.md
    │       │   └── SKILL.md
    │       ├── aif-roadmap
    │       │   └── SKILL.md
    │       ├── aif-rules
    │       │   └── SKILL.md
    │       ├── aif-rules-check
    │       │   ├── references
    │       │   │   └── RULES-CHECK-CONTRACT.md
    │       │   └── SKILL.md
    │       ├── aif-security-checklist
    │       │   ├── references
    │       │   │   ├── AUTH-PATTERNS.md
    │       │   │   ├── PROMPT-INJECTION.md
    │       │   │   └── RACE-CONDITIONS.md
    │       │   ├── scripts
    │       │   │   └── audit.sh
    │       │   └── SKILL.md
    │       ├── aif-skill-generator
    │       │   ├── references
    │       │   │   ├── BEST-PRACTICES.md
    │       │   │   ├── EXAMPLES.md
    │       │   │   ├── LEARN-MODE.md
    │       │   │   ├── SECURITY-SCANNING.md
    │       │   │   └── SPECIFICATION.md
    │       │   ├── scripts
    │       │   │   ├── cleanup-blocked-skill.py
    │       │   │   ├── search-skills.py
    │       │   │   ├── security-scan.py
    │       │   │   └── validate.sh
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       ├── basic.md
    │       │       ├── dynamic-context.md
    │       │       ├── research.md
    │       │       ├── task.md
    │       │       └── visual.md
    │       ├── aif-verify
    │       │   ├── references
    │       │   │   ├── CONTEXT-GATES-AND-OWNERSHIP.md
    │       │   │   └── GATE-RESULT-CONTRACT.md
    │       │   └── SKILL.md
    │       ├── golang-code-style
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   └── details.md
    │       │   └── SKILL.md
    │       ├── golang-concurrency
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── channels-and-select.md
    │       │   │   ├── pipelines.md
    │       │   │   └── sync-primitives.md
    │       │   └── SKILL.md
    │       ├── golang-context
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── cancellation.md
    │       │   │   ├── http-services.md
    │       │   │   └── values-tracing.md
    │       │   └── SKILL.md
    │       ├── golang-database
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── performance.md
    │       │   │   ├── scanning.md
    │       │   │   ├── testing.md
    │       │   │   └── transactions.md
    │       │   └── SKILL.md
    │       ├── golang-design-patterns
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── architecture.md
    │       │   │   ├── clean-architecture.md
    │       │   │   ├── data-handling.md
    │       │   │   ├── ddd.md
    │       │   │   ├── hexagonal-architecture.md
    │       │   │   └── resource-management.md
    │       │   └── SKILL.md
    │       ├── golang-documentation
    │       │   ├── assets
    │       │   │   └── templates
    │       │   │       ├── CHANGELOG.md
    │       │   │       ├── CONTRIBUTING.md
    │       │   │       ├── llms.txt
    │       │   │       └── README.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── application.md
    │       │   │   ├── code-comments.md
    │       │   │   ├── library.md
    │       │   │   └── project-docs.md
    │       │   └── SKILL.md
    │       ├── golang-error-handling
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── error-creation.md
    │       │   │   ├── error-handling.md
    │       │   │   └── error-wrapping.md
    │       │   └── SKILL.md
    │       ├── golang-naming
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── functions-methods.md
    │       │   │   ├── identifiers.md
    │       │   │   ├── packages-files.md
    │       │   │   ├── testing.md
    │       │   │   └── types-errors.md
    │       │   └── SKILL.md
    │       ├── golang-performance
    │       │   ├── assets
    │       │   │   └── prometheus-alerts.yml
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── caching.md
    │       │   │   ├── cpu.md
    │       │   │   ├── io-networking.md
    │       │   │   ├── memory.md
    │       │   │   ├── observability.md
    │       │   │   └── runtime.md
    │       │   └── SKILL.md
    │       ├── golang-security
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── architecture.md
    │       │   │   ├── checklist.md
    │       │   │   ├── cookies.md
    │       │   │   ├── cryptography.md
    │       │   │   ├── filesystem.md
    │       │   │   ├── injection.md
    │       │   │   ├── logging.md
    │       │   │   ├── memory-safety.md
    │       │   │   ├── network.md
    │       │   │   ├── secrets.md
    │       │   │   ├── third-party.md
    │       │   │   └── threat-modeling.md
    │       │   └── SKILL.md
    │       └── golang-testing
    │           ├── evals
    │           │   └── evals.json
    │           ├── references
    │           │   ├── benchmarks.md
    │           │   ├── coverage.md
    │           │   ├── examples.md
    │           │   ├── helpers.md
    │           │   ├── http-testing.md
    │           │   ├── integration-testing.md
    │           │   └── mocking.md
    │           └── SKILL.md
    ├── AGENTS.md
    ├── .ai-factory
    │   ├── ARCHITECTURE.md
    │   ├── config.yaml
    │   ├── DESCRIPTION.md
    │   ├── plans
    │   │   └── feature-restructure-into-internal-packages
    │   │       ├── HANDOFF.md
    │   │       ├── index.md
    │   │       ├── phase-01-baseline-and-barriers.md
    │   │       ├── phase-02-repository-root.md
    │   │       ├── phase-03-subsystems.md
    │   │       ├── phase-04-shared-primitives.md
    │   │       ├── phase-05-leaf-packages.md
    │   │       ├── phase-06-hosts-and-services.md
    │   │       ├── phase-07-view-and-terminal.md
    │   │       ├── phase-08-fileops-and-editor.md
    │   │       ├── phase-09-panel-and-cmdline.md
    │   │       ├── phase-10-composition-root.md
    │   │       ├── phase-11-ci-and-docs.md
    │   │       └── PR-BODY.md
    │   └── rules
    │       └── base.md
    ├── .ai-factory.json
    ├── artifacts
    │   ├── native-openconsole-probe.json
    │   ├── native-openconsole-probe.json.sessions
    │   │   ├── 121x40.raw
    │   │   ├── 1x1.raw
    │   │   └── 80x25.raw
    │   ├── native-openconsole-probe-static.json
    │   ├── native-openconsole-probe-static.json.sessions
    │   │   └── 80x25.raw
    │   └── README.md
    ├── cmd
    │   └── f4
    │       ├── architecture_test.go
    │       ├── command_palette_coverage_test.go
    │       ├── frame_manager_capture_test.go
    │       ├── hardcoded_strings_test.go
    │       ├── main.go
    │       ├── rsrc_windows_amd64.syso
    │       └── rsrc_windows_arm64.syso
    ├── docs
    │   ├── COLORS.md
    │   ├── CONPTY_FUTURE_IDEAS.md
    │   ├── CONPTY_GATE_REQUIREMENTS.md
    │   ├── CONPTY_GATE_STATUS.md
    │   ├── CONPTY_LINE_WRAP_FINDINGS.md
    │   ├── CONPTY_NATIVE_AGENT.md
    │   ├── CONPTY_NATIVE_AUDIT.md
    │   ├── CONPTY_NATIVE_PROBE.md
    │   ├── CONPTY_NATIVE_TEST.md
    │   ├── CONSOLE_MODES.md
    │   ├── CURSOR.md
    │   ├── DRAGDROP.md
    │   ├── FFI.md
    │   ├── FISH+.md
    │   ├── FISH_PLUS_S2S.md
    │   ├── FUSE.md
    │   ├── HIGHLIGHTING.md
    │   ├── HIGHLIGHT.md
    │   ├── I18N.md
    │   ├── IDEAS.md
    │   ├── IMAGES_PLAN.md
    │   ├── issue-561-solutions.md
    │   ├── ISSUES
    │   │   ├── ISSUE_1179_7Z_SFX_DETECTION_AND_TESTING.md
    │   │   ├── ISSUE_165_CONPTY_SYNC_MARKER.md
    │   │   ├── ISSUE_215_WINDOWS_UPDATE_ELEVATION.md
    │   │   ├── ISSUE_247_QUEUE_PROGRESS_THROTTLE.md
    │   │   ├── ISSUE_248_PTY_REDRAW_COALESCING.md
    │   │   ├── ISSUE_260_SUDO_ARGUMENT_INHERITANCE.md
    │   │   ├── ISSUE_261_COLOR_AREAS.md
    │   │   ├── ISSUE_262_REMOTE_VFS_PATHS.md
    │   │   ├── ISSUE_264_UPDATE_CHECK_VERSIONS.md
    │   │   ├── ISSUE_266_MULTI_SELECTION_ATTRIBUTES.md
    │   │   ├── ISSUE_277_COLORER_FEATURES.md
    │   │   ├── ISSUE_278_SORT_ORDER_COLLATION.md
    │   │   ├── ISSUE_309_PLUGRING_ASSET_URLS.md
    │   │   ├── ISSUE_397_WINDOWS_CONSOLE_LAYOUT_CRASH.md
    │   │   ├── ISSUE_411_HOTKEY_DIALOG_COMMIT.md
    │   │   ├── ISSUE_453_OVERWRITE_DIALOG_LOCALIZATION.md
    │   │   ├── ISSUE_492_RIGHT_CTRL_BINDINGS.md
    │   │   ├── ISSUE_493_HOTKEY_LIST_SELECTION.md
    │   │   ├── ISSUE_511_MULTIPLEXER_CHORDS.md
    │   │   ├── ISSUE_523_GZIP_LOGICAL_FILE.md
    │   │   ├── ISSUE_526_COMMAND_LINE_QUOTING.md
    │   │   ├── ISSUE_546_VIEWER_GRAPHEME_CLUSTERS_FOLLOWUP.md
    │   │   ├── ISSUE_546_VIEWER_GRAPHEME_CLUSTERS.md
    │   │   ├── ISSUE_601_STARTUP_BACKEND_SELECTION.md
    │   │   ├── ISSUE_606_FUSE_LOCK_GRANULARITY.md
    │   │   ├── ISSUE_608_COPY_DIALOG_LAYOUT.md
    │   │   ├── ISSUE_651_MENU_OVERFLOW_CHECKLIST.md
    │   │   ├── ISSUE_677_ATOMIC_CONFIG_WRITES.md
    │   │   ├── ISSUE_689_EDITOR_SYNTAX_FADE.md
    │   │   ├── ISSUE_693_STATIC_LINUX_FFI.md
    │   │   ├── ISSUE_703_COPY_WINDOW_TITLE_FOLLOWUP.md
    │   │   ├── ISSUE_703_COPY_WINDOW_TITLE.md
    │   │   ├── ISSUE_722_COPY_ACCESS_RIGHTS.md
    │   │   ├── ISSUE_722_COPY_OPTIONS.md
    │   │   ├── ISSUE_724_SSH_HOST_KEY_VERIFICATION.md
    │   │   ├── ISSUE_725_SSH_AGENT_FORWARDING.md
    │   │   ├── ISSUE_727_S3_UPLOAD_API.md
    │   │   ├── ISSUE_744_CI_MERGED_RUN_FAILURES.md
    │   │   ├── ISSUE_793_MENU_COMMAND_PALETTE_ITEM.md
    │   │   ├── ISSUE_807_BOOKMARKS_HOTKEY.md
    │   │   ├── ISSUE_816_ARCHIVE_PASSWORD_DIALOG_FOLLOWUP.md
    │   │   ├── ISSUE_833_CODEPAGE_AUTODETECTION.md
    │   │   ├── ISSUE_834_FILE_TITLE_DISAMBIGUATION.md
    │   │   ├── ISSUE_87_NESTED_TERMINAL_INPUT.md
    │   │   ├── ISSUE_915_ARCHIVE_TEST_TOTAL_PROGRESS.md
    │   │   ├── ISSUE_91_FREEBSD_CONSOLE_DIAGNOSIS.md
    │   │   ├── ISSUE_95_TERMINAL_TAB_COMPLETION_FOLLOWUP.md
    │   │   └── ISSUE_95_TERMINAL_TAB_COMPLETION.md
    │   ├── KEYMAP.md
    │   ├── L10N_REPORT_GUIDE.md
    │   ├── LUA.md
    │   ├── MACKEYS.md
    │   ├── MACROS.md
    │   ├── PINNED_CONSOLE.md
    │   ├── PINNED_HOST_FACTS.md
    │   ├── PLAYER.md
    │   ├── PLUGIN_PLAN.md
    │   ├── PLUGINS.md
    │   ├── PLUGRING.md
    │   ├── PORTABILITY_BSD.md
    │   ├── PROMPT.md
    │   ├── REVIEW.md
    │   ├── settings-center-fields.json
    │   ├── SETTINGS_CENTER.md
    │   ├── SPREADSHEET.md
    │   ├── SYNC_DIRS.md
    │   ├── TERMINAL.md
    │   ├── TEST_OPTIMIZATION_PLAN.md
    │   ├── TTYX.md
    │   ├── USER_MENU.md
    │   ├── UX_GUIDELINES.md
    │   ├── VFS.md
    │   ├── VIDEO.md
    │   ├── VTML.md
    │   ├── VTVIBE.md
    │   ├── WINCON_805_HANDOVER.md
    │   ├── WINCON.md
    │   └── WINE.md
    ├── embedded.go
    ├── f4.example.ini
    ├── .gitattributes
    ├── .github
    │   ├── actions
    │   │   ├── affected-packages
    │   │   │   └── action.yml
    │   │   └── shard-packages
    │   │       └── action.yml
    │   ├── assets
    │   │   └── screenshot.png
    │   ├── codecov.yml
    │   └── workflows
    │       ├── build.yml
    │       ├── go-cache-salt
    │       └── quick.yml
    ├── .gitignore
    ├── .golangci-strict.yml
    ├── .golangci.yml
    ├── go.mod
    ├── go.sum
    ├── highlight.ini
    ├── internal
    │   ├── action
    │   │   ├── order.go
    │   │   ├── registry.go
    │   │   └── registry_order_test.go
    │   ├── app
    │   │   ├── action_copyname_parent_test.go
    │   │   ├── action_copy_window_title_test.go
    │   │   ├── action_marked_clipboard_test.go
    │   │   ├── action_menu.go
    │   │   ├── action_menu_test.go
    │   │   ├── action_menu_visibility_test.go
    │   │   ├── action_registry_test.go
    │   │   ├── action_restore_selection_test.go
    │   │   ├── actions_coverage_helpers_test.go
    │   │   ├── actions_framework.go
    │   │   ├── actions_framework_test.go
    │   │   ├── actions.go
    │   │   ├── action_shortcut_conflict_test.go
    │   │   ├── actions_table.go
    │   │   ├── actions_table_order_test.go
    │   │   ├── actions_test.go
    │   │   ├── actions_view_by_type_test.go
    │   │   ├── ai_chat_panel.go
    │   │   ├── ai_chat_panel_test.go
    │   │   ├── api.go
    │   │   ├── api_test.go
    │   │   ├── apply_command_test.go
    │   │   ├── arkanoid_coverage_test.go
    │   │   ├── arkanoid.go
    │   │   ├── arkanoid_test.go
    │   │   ├── attributes_test.go
    │   │   ├── autosave_settings_test.go
    │   │   ├── background_jobs_window.go
    │   │   ├── background_jobs_window_test.go
    │   │   ├── bom_test.go
    │   │   ├── bookmarks_dialog_test.go
    │   │   ├── bookmarks_test.go
    │   │   ├── bootstrap_backend.go
    │   │   ├── bootstrap_backend_test.go
    │   │   ├── bootstrap_coverage_test.go
    │   │   ├── bootstrap_ctrlhandler_other.go
    │   │   ├── bootstrap_ctrlhandler_windows.go
    │   │   ├── bootstrap_ctrlhandler_windows_test.go
    │   │   ├── bootstrap_detach_unix.go
    │   │   ├── bootstrap_detach_unix_test.go
    │   │   ├── bootstrap_detach_windows.go
    │   │   ├── bootstrap_detach_windows_test.go
    │   │   ├── bootstrap.go
    │   │   ├── bootstrap_gui_test.go
    │   │   ├── bootstrap_nestedinput_other.go
    │   │   ├── bootstrap_nestedinput_test.go
    │   │   ├── bootstrap_nestedinput_windows.go
    │   │   ├── bootstrap_nestedinput_windows_test.go
    │   │   ├── bootstrap_session_test.go
    │   │   ├── bootstrap_settings.go
    │   │   ├── bootstrap_settings_test.go
    │   │   ├── bootstrap_startupdir_terminal_test.go
    │   │   ├── bootstrap_startupdir_test.go
    │   │   ├── bootstrap_startupfile_terminal_test.go
    │   │   ├── bootstrap_sudo_test.go
    │   │   ├── bootstrap_unicode_test.go
    │   │   ├── child_env_test.go
    │   │   ├── child_env_universal_linux_test.go
    │   │   ├── cloudfox_real_archive_test.go
    │   │   ├── cloudfox_real_cross_cloud_test.go
    │   │   ├── cloudfox_real_large_f5_test.go
    │   │   ├── cloudfox_real_ui_test.go
    │   │   ├── codepage_issue875_sticky_test.go
    │   │   ├── codepage_issue875_test.go
    │   │   ├── colorer_download_test.go
    │   │   ├── colorer_settings.go
    │   │   ├── colorer_settings_test.go
    │   │   ├── colorer_type_settings.go
    │   │   ├── command_history_paths_test.go
    │   │   ├── command_palette_direct_frames.go
    │   │   ├── command_palette_direct_frames_test.go
    │   │   ├── command_palette_direct_panels_test.go
    │   │   ├── command_palette_drives.go
    │   │   ├── command_palette_drives_test.go
    │   │   ├── command_palette_dynamic_test.go
    │   │   ├── command_palette_frames.go
    │   │   ├── command_palette.go
    │   │   ├── command_palette_help.go
    │   │   ├── command_palette_help_test.go
    │   │   ├── command_palette_i18n.go
    │   │   ├── command_palette_i18n_test.go
    │   │   ├── command_palette_macros.go
    │   │   ├── command_palette_menu_leaves_test.go
    │   │   ├── command_palette_menu_test.go
    │   │   ├── command_palette_modal.go
    │   │   ├── command_palette_panels.go
    │   │   ├── command_palette_prefixes.go
    │   │   ├── command_palette_search.go
    │   │   ├── command_palette_search_test.go
    │   │   ├── command_palette_test.go
    │   │   ├── command_palette_ui.go
    │   │   ├── command_palette_ui_test.go
    │   │   ├── command_palette_workspace.go
    │   │   ├── command_prefix_registry_test.go
    │   │   ├── compare_folders_ui_coverage_test.go
    │   │   ├── compare_folders_ui.go
    │   │   ├── config_test.go
    │   │   ├── console_passthrough_test.go
    │   │   ├── copy_dialog_options.go
    │   │   ├── coverage_helpers_test.go
    │   │   ├── debug.go
    │   │   ├── debug_hangdump_unix.go
    │   │   ├── debug_hangdump_windows.go
    │   │   ├── debug_log_test.go
    │   │   ├── delete_trash_test.go
    │   │   ├── dialog_copy_resize_test.go
    │   │   ├── dialog_layout_languages_test.go
    │   │   ├── dialog_layouts_test.go
    │   │   ├── disasm_editor_test.go
    │   │   ├── dragdrop_test.go
    │   │   ├── drive_bookmarks_test.go
    │   │   ├── drive_menu_options_test.go
    │   │   ├── editor_actions_test.go
    │   │   ├── editor_binary_open_test.go
    │   │   ├── editor_host_test.go
    │   │   ├── editor_hotkeys_test.go
    │   │   ├── editor_keys_test.go
    │   │   ├── editor_save_wait_test.go
    │   │   ├── editor_test_helpers_test.go
    │   │   ├── f4_commands.go
    │   │   ├── f4_commands_test.go
    │   │   ├── farmenu_file_test.go
    │   │   ├── fast_find_overlay_test.go
    │   │   ├── file_associations_dispatch_test.go
    │   │   ├── file_associations_test.go
    │   │   ├── file_ops_panel_test.go
    │   │   ├── file_panel_sorting_regression_test.go
    │   │   ├── find_file.go
    │   │   ├── find_file_test.go
    │   │   ├── fkeys_hidden_panels_test.go
    │   │   ├── folder_history_actions_test.go
    │   │   ├── folder_history_navigation_test.go
    │   │   ├── folder_history_panel_test.go
    │   │   ├── grabber.go
    │   │   ├── grabber_mouse_test.go
    │   │   ├── grabber_test.go
    │   │   ├── gui_font_combo_dialog_test.go
    │   │   ├── help_host_test.go
    │   │   ├── help_keys_ar_test.go
    │   │   ├── help_keys_he_test.go
    │   │   ├── help_keys_ru_test.go
    │   │   ├── help_keys_test.go
    │   │   ├── help_keys_tr_test.go
    │   │   ├── help_topics.go
    │   │   ├── history_bridge.go
    │   │   ├── history_dialog.go
    │   │   ├── history_dialog_test.go
    │   │   ├── history_hint_test.go
    │   │   ├── history_provider_test.go
    │   │   ├── hotkeys_ui.go
    │   │   ├── hotkeys_ui_test.go
    │   │   ├── image_gallery_panel_test.go
    │   │   ├── image_view_panel_test.go
    │   │   ├── issue54_test.go
    │   │   ├── issue561_test.go
    │   │   ├── issue631_test.go
    │   │   ├── issue821_test.go
    │   │   ├── issue856_mouse_capture_test.go
    │   │   ├── issue95_followup_test.go
    │   │   ├── keybar_injected_test.go
    │   │   ├── keymap_host_test.go
    │   │   ├── keymap_suspend.go
    │   │   ├── lang.go
    │   │   ├── lang_host_test.go
    │   │   ├── local_language_files_test.go
    │   │   ├── macro_ctrlletter_test.go
    │   │   ├── macro_dispatch.go
    │   │   ├── macro_host.go
    │   │   ├── macro_host_test.go
    │   │   ├── macro_plugin_calls.go
    │   │   ├── macro_reload_action_test.go
    │   │   ├── macro_test.go
    │   │   ├── main_menu_bar_keys_test.go
    │   │   ├── main_menu_dropdown_test.go
    │   │   ├── main_test.go
    │   │   ├── managed_execution_test.go
    │   │   ├── media_app.go
    │   │   ├── menu_history_action.go
    │   │   ├── menu_history_test.go
    │   │   ├── mock_failing_vfs_test.go
    │   │   ├── navigation_mode_test.go
    │   │   ├── panel_actions_test.go
    │   │   ├── panel_menu_test.go
    │   │   ├── panel_plugins_test.go
    │   │   ├── panels_app_commands_coverage_test.go
    │   │   ├── panels_app_commands.go
    │   │   ├── panels_frame_app_test.go
    │   │   ├── panels_frame_drivecursor_windows_test.go
    │   │   ├── path_hints_test.go
    │   │   ├── path_identity_history_test.go
    │   │   ├── plughost_app.go
    │   │   ├── plugin_contributions.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── plugin_hotkeys_test.go
    │   │   ├── plugring_policy_test.go
    │   │   ├── plugring_rows_test.go
    │   │   ├── plugring_test.go
    │   │   ├── plugring_ui.go
    │   │   ├── plugring_ui_test.go
    │   │   ├── portable_paths_test.go
    │   │   ├── portable_test.go
    │   │   ├── process_environment_host.go
    │   │   ├── process_environment_host_test.go
    │   │   ├── pty_windows_panel_test.go
    │   │   ├── quickview_api.go
    │   │   ├── rpc_commands_test.go
    │   │   ├── runner_unix_panel_test.go
    │   │   ├── search_history_test.go
    │   │   ├── semantic_actions_test.go
    │   │   ├── semantic.go
    │   │   ├── semantic_test.go
    │   │   ├── settings_host.go
    │   │   ├── settings_routes.go
    │   │   ├── settings_save.go
    │   │   ├── settings_save_route_test.go
    │   │   ├── share_dialog_coverage_test.go
    │   │   ├── share_dialog.go
    │   │   ├── share_dialog_test.go
    │   │   ├── sheet_actions.go
    │   │   ├── sheet_actions_test.go
    │   │   ├── sheet_compare_coverage_test.go
    │   │   ├── sheet_dialogs_coverage_test.go
    │   │   ├── sheet_dialogs.go
    │   │   ├── sheet_frame.go
    │   │   ├── sheet_frame_test.go
    │   │   ├── sheet_palette.go
    │   │   ├── sheet_palette_test.go
    │   │   ├── shell_integration_test.go
    │   │   ├── shutdown_cancel_test.go
    │   │   ├── simple_exec_test.go
    │   │   ├── sort_groups_test.go
    │   │   ├── sqlite_actions.go
    │   │   ├── sqlite_actions_test.go
    │   │   ├── static_direct_actions.go
    │   │   ├── static_direct_actions_test.go
    │   │   ├── sync_dirs_ui.go
    │   │   ├── temp_panel_test.go
    │   │   ├── term_app.go
    │   │   ├── term_app_other.go
    │   │   ├── term_app_windows.go
    │   │   ├── terminal_workspace_test.go
    │   │   ├── testdata
    │   │   │   └── action_order.golden
    │   │   ├── text_editor_bridge_test.go
    │   │   ├── theme_host_test.go
    │   │   ├── title.go
    │   │   ├── title_test.go
    │   │   ├── title_unix.go
    │   │   ├── title_windows.go
    │   │   ├── translator_test.go
    │   │   ├── updater.go
    │   │   ├── updater_issue635_test.go
    │   │   ├── updater_repro_lock_other_test.go
    │   │   ├── updater_repro_lock_windows_test.go
    │   │   ├── updater_repro_test.go
    │   │   ├── updater_test.go
    │   │   ├── user_menu_ini_test.go
    │   │   ├── user_menu_subst_test.go
    │   │   ├── viewer_app.go
    │   │   ├── viewer_editor_history_test.go
    │   │   ├── viewer_keys_test.go
    │   │   ├── vtvibe_ap_coverage_test.go
    │   │   ├── vtvibe_ap.go
    │   │   ├── vtvibe_ap_test.go
    │   │   ├── vtvibe_host_coverage_test.go
    │   │   ├── vtvibe_host.go
    │   │   ├── vtvibe_host_test.go
    │   │   ├── win32_backend_test.go
    │   │   ├── workspace_routing_test.go
    │   │   ├── workspace_session_test.go
    │   │   ├── worktree_identity.go
    │   │   └── worktree_identity_test.go
    │   ├── appcmd
    │   │   └── commands.go
    │   ├── cmdline
    │   │   ├── apply_batch.go
    │   │   ├── apply_batch_test.go
    │   │   ├── apply_output.go
    │   │   ├── apply_output_test.go
    │   │   ├── apply_resources.go
    │   │   ├── apply_resources_test.go
    │   │   ├── apply_shortname_other.go
    │   │   ├── apply_shortname_windows.go
    │   │   ├── apply_shortname_windows_test.go
    │   │   ├── apply_subst.go
    │   │   ├── apply_subst_test.go
    │   │   ├── apply_transcript.go
    │   │   ├── line.go
    │   │   ├── line_semantic.go
    │   │   ├── line_semantic_test.go
    │   │   ├── line_test.go
    │   │   ├── prompt.go
    │   │   ├── prompt_test.go
    │   │   ├── prompt_unix.go
    │   │   ├── prompt_windows.go
    │   │   ├── quotes.go
    │   │   ├── quotes_test.go
    │   │   ├── quoting.go
    │   │   ├── quoting_test.go
    │   │   ├── resolve_other.go
    │   │   ├── resolve_windows.go
    │   │   └── resolve_windows_test.go
    │   ├── colorer
    │   │   ├── configs
    │   │   │   └── base
    │   │   │       └── hrd
    │   │   │           └── rgb
    │   │   │               └── radiola.hrd
    │   │   └── embedded.go
    │   ├── config
    │   │   ├── appearance_settings_test.go
    │   │   ├── atomic.go
    │   │   ├── atomic_test.go
    │   │   ├── config.go
    │   │   ├── config_test.go
    │   │   ├── cursor_style_test.go
    │   │   ├── fallback_language_test.go
    │   │   ├── options.go
    │   │   ├── options_test.go
    │   │   ├── overlay.go
    │   │   ├── overlay_test.go
    │   │   ├── proxy_settings_test.go
    │   │   ├── schema.go
    │   │   ├── serialize_roundtrip_test.go
    │   │   └── settings.go
    │   ├── dialog
    │   │   ├── about.go
    │   │   ├── about_os_other.go
    │   │   ├── about_os_unix.go
    │   │   ├── about_os_windows.go
    │   │   ├── about_test.go
    │   │   ├── attributes.go
    │   │   ├── attributes_mixed_test.go
    │   │   ├── attributes_unix.go
    │   │   ├── attributes_windows.go
    │   │   ├── attributes_windows_test.go
    │   │   ├── caption.go
    │   │   ├── config_editor.go
    │   │   ├── config_editor_test.go
    │   │   ├── envman_help_test.go
    │   │   ├── file.go
    │   │   ├── file_resize_test.go
    │   │   ├── file_test.go
    │   │   ├── goto.go
    │   │   ├── help
    │   │   │   ├── ar.hlf
    │   │   │   ├── be.hlf
    │   │   │   ├── bn.hlf
    │   │   │   ├── cs.hlf
    │   │   │   ├── de.hlf
    │   │   │   ├── en.hlf
    │   │   │   ├── es.hlf
    │   │   │   ├── et.hlf
    │   │   │   ├── fi.hlf
    │   │   │   ├── he.hlf
    │   │   │   ├── hi.hlf
    │   │   │   ├── hu.hlf
    │   │   │   ├── hy.hlf
    │   │   │   ├── ja.hlf
    │   │   │   ├── ka.hlf
    │   │   │   ├── ko.hlf
    │   │   │   ├── lt.hlf
    │   │   │   ├── lv.hlf
    │   │   │   ├── pl.hlf
    │   │   │   ├── README.md
    │   │   │   ├── ru.hlf
    │   │   │   ├── tr.hlf
    │   │   │   ├── uk.hlf
    │   │   │   └── zh.hlf
    │   │   ├── help.go
    │   │   ├── help_lang_test.go
    │   │   ├── help_languages.go
    │   │   ├── help_search.go
    │   │   ├── help_search_test.go
    │   │   ├── help_test.go
    │   │   ├── hotkey_capture.go
    │   │   ├── label.go
    │   │   ├── main_test.go
    │   │   ├── path.go
    │   │   ├── profile_transfer_test.go
    │   │   ├── settings_codepage.go
    │   │   ├── settings_portable.go
    │   │   ├── settings_portable_test.go
    │   │   ├── settings_proxy_coverage_test.go
    │   │   ├── settings_proxy.go
    │   │   └── settings_proxy_test.go
    │   ├── editor
    │   │   ├── base64.go
    │   │   ├── buffer_async.go
    │   │   ├── buffer_async_test.go
    │   │   ├── buffer_mapped.go
    │   │   ├── buffer_mapped_unix.go
    │   │   ├── buffer_mapped_windows.go
    │   │   ├── colorer_async.go
    │   │   ├── colorer_check.go
    │   │   ├── colorer_check_test.go
    │   │   ├── colorer_cpu_amd64.go
    │   │   ├── colorer_cpu_amd64_test.go
    │   │   ├── colorer_cpu.go
    │   │   ├── colorer_cpu_other.go
    │   │   ├── colorer_cpu_test.go
    │   │   ├── colorer_diagnostics_test.go
    │   │   ├── colorer_downloader.go
    │   │   ├── colorer.go
    │   │   ├── colorer_outline_frame.go
    │   │   ├── colorer_outline_frame_test.go
    │   │   ├── colorer_outline.go
    │   │   ├── colorer_outline_test.go
    │   │   ├── colorer_pair_search.go
    │   │   ├── colorer_pairs.go
    │   │   ├── colorer_pairs_test.go
    │   │   ├── colorer_params.go
    │   │   ├── colorer_params_test.go
    │   │   ├── colorer_plugin_test.go
    │   │   ├── colorer_reload.go
    │   │   ├── colorer_text.go
    │   │   ├── colorer_type_settings.go
    │   │   ├── colorer_type_settings_test.go
    │   │   ├── colorer_types.go
    │   │   ├── colorer_types_test.go
    │   │   ├── colorer_window.go
    │   │   ├── editor_base64_test.go
    │   │   ├── editor_codepage_test.go
    │   │   ├── editor_delta_test.go
    │   │   ├── editor_duplicate_line_test.go
    │   │   ├── editor_fade_test.go
    │   │   ├── editor_features_test.go
    │   │   ├── editor_find_all_test.go
    │   │   ├── editor_highlight_budget_test.go
    │   │   ├── editor_index_status_test.go
    │   │   ├── editor_mmap_test.go
    │   │   ├── editor_move_line_test.go
    │   │   ├── editor_multicursor_edit_test.go
    │   │   ├── editor_multicursor_move_test.go
    │   │   ├── editor_multicursor_occurrence_test.go
    │   │   ├── editor_multicursor_select_test.go
    │   │   ├── editor_multicursor_test.go
    │   │   ├── editor_occurrence_test.go
    │   │   ├── editor_restore_keys_test.go
    │   │   ├── editor_save_as_test.go
    │   │   ├── editor_save_inplace_test.go
    │   │   ├── editor_search_lazy_test.go
    │   │   ├── editor_search_remote_test.go
    │   │   ├── editor_search_zerocopy_test.go
    │   │   ├── editor_shiftdel_test.go
    │   │   ├── editor_target_line_test.go
    │   │   ├── editor_veto_test.go
    │   │   ├── editor_view_ads_test.go
    │   │   ├── editor_view_test.go
    │   │   ├── editor_wrap_memory_test.go
    │   │   ├── editor_wrap_safety_test.go
    │   │   ├── escape_colorer_test.go
    │   │   ├── external_command.go
    │   │   ├── external_editor_process_unix_test.go
    │   │   ├── external_editor_test.go
    │   │   ├── external_freebsd.go
    │   │   ├── external_freebsd_test.go
    │   │   ├── external_unix.go
    │   │   ├── external_windows.go
    │   │   ├── fade.go
    │   │   ├── findall.go
    │   │   ├── goto_test.go
    │   │   ├── grapheme.go
    │   │   ├── host.go
    │   │   ├── index_status.go
    │   │   ├── main_test.go
    │   │   ├── mapped_file_test.go
    │   │   ├── menubar_test.go
    │   │   ├── multicursor.go
    │   │   ├── replace_confirm.go
    │   │   ├── save_as.go
    │   │   ├── search_remote.go
    │   │   ├── sort.go
    │   │   ├── status.go
    │   │   ├── url_links_hover_test.go
    │   │   ├── view.go
    │   │   ├── view_semantic.go
    │   │   └── wrap_safety.go
    │   ├── fileops
    │   │   ├── access_rights.go
    │   │   ├── access_rights_test.go
    │   │   ├── archive_index_fallback.go
    │   │   ├── archive_index.go
    │   │   ├── archive_index_test.go
    │   │   ├── buttons.go
    │   │   ├── buttons_test.go
    │   │   ├── codepage.go
    │   │   ├── compare_folders_test.go
    │   │   ├── compare.go
    │   │   ├── copy_options.go
    │   │   ├── copy_options_test.go
    │   │   ├── dialog_reporter_test.go
    │   │   ├── file_mask_far2l_test.go
    │   │   ├── file_mask_test.go
    │   │   ├── file_op_dialog_test.go
    │   │   ├── file_ops_coverage_test.go
    │   │   ├── file_ops_safety_test.go
    │   │   ├── file_ops_test.go
    │   │   ├── file_ops_transfer_name_test.go
    │   │   ├── file_op_tracker_test.go
    │   │   ├── host.go
    │   │   ├── host_test.go
    │   │   ├── identity.go
    │   │   ├── issue149_test.go
    │   │   ├── issue815_test.go
    │   │   ├── local.go
    │   │   ├── main_test.go
    │   │   ├── mask.go
    │   │   ├── ops_dialog.go
    │   │   ├── ops.go
    │   │   ├── path_identity_test.go
    │   │   ├── pump.go
    │   │   ├── queue.go
    │   │   ├── queue_manager_test.go
    │   │   ├── report.go
    │   │   ├── rights_extra.go
    │   │   ├── security_other.go
    │   │   ├── security_windows.go
    │   │   ├── security_windows_test.go
    │   │   ├── state.go
    │   │   ├── state_key_test.go
    │   │   ├── state_test.go
    │   │   ├── symlink.go
    │   │   ├── sync.go
    │   │   ├── sync_test.go
    │   │   └── tracker.go
    │   ├── fusefs
    │   │   ├── bench-all.sh
    │   │   ├── BENCH.md
    │   │   ├── bench.sh
    │   │   ├── bridge.go
    │   │   ├── bridge_test.go
    │   │   ├── cli.go
    │   │   ├── cli_test.go
    │   │   ├── fusefs.go
    │   │   ├── FUSE.md
    │   │   ├── mountspec.go
    │   │   ├── node_fuse.go
    │   │   ├── node_fuse_test.go
    │   │   ├── node_unsupported.go
    │   │   ├── platform_other.go
    │   │   ├── platform_unix.go
    │   │   ├── registry.go
    │   │   ├── staged_test.go
    │   │   └── writers_test.go
    │   ├── gui
    │   │   ├── assets
    │   │   │   └── icon
    │   │   │       ├── f4-16.svg
    │   │   │       ├── f4-24.svg
    │   │   │       ├── f4-30.svg
    │   │   │       ├── f4-32.svg
    │   │   │       ├── f4-36.svg
    │   │   │       ├── f4-42.svg
    │   │   │       ├── f4.svg
    │   │   │       ├── generated
    │   │   │       │   ├── f4-1024.png
    │   │   │       │   ├── f4-128.png
    │   │   │       │   ├── f4-16.png
    │   │   │       │   ├── f4-24.png
    │   │   │       │   ├── f4-256.png
    │   │   │       │   ├── f4-28.png
    │   │   │       │   ├── f4-30.png
    │   │   │       │   ├── f4-32.png
    │   │   │       │   ├── f4-36.png
    │   │   │       │   ├── f4-42.png
    │   │   │       │   ├── f4-48.png
    │   │   │       │   ├── f4-512.png
    │   │   │       │   ├── f4-56.png
    │   │   │       │   ├── f4-64.png
    │   │   │       │   ├── f4.icns
    │   │   │       │   └── f4.ico
    │   │   │       └── README.md
    │   │   ├── backend_ffi.go
    │   │   ├── backend.go
    │   │   ├── backend_stub.go
    │   │   ├── backend_test.go
    │   │   ├── font_catalog.go
    │   │   ├── font_catalog_test.go
    │   │   ├── font_catalog_unix.go
    │   │   ├── font_catalog_windows.go
    │   │   ├── font_combo.go
    │   │   ├── font.go
    │   │   ├── font_notwindows.go
    │   │   ├── font_test.go
    │   │   ├── font_windows.go
    │   │   ├── font_windows_test.go
    │   │   ├── icon_darwin.go
    │   │   ├── icon_darwin_test.go
    │   │   ├── icon_unix.go
    │   │   ├── icon_windows.go
    │   │   ├── icon_windows_test.go
    │   │   ├── runtime_mode.go
    │   │   ├── run_unix.go
    │   │   ├── run_unix_test.go
    │   │   ├── run_windows.go
    │   │   ├── window.go
    │   │   ├── winepath_other.go
    │   │   ├── winepath_windows.go
    │   │   └── winepath_windows_test.go
    │   ├── hideconsole
    │   │   ├── go.mod
    │   │   └── hideconsole.go
    │   ├── history
    │   │   ├── edit.go
    │   │   ├── far2l.go
    │   │   ├── menu.go
    │   │   ├── paths.go
    │   │   ├── pins.go
    │   │   └── provider.go
    │   ├── i18n
    │   │   ├── lang
    │   │   │   ├── ar.lng
    │   │   │   ├── be.lng
    │   │   │   ├── bn.lng
    │   │   │   ├── coverage_baseline.txt
    │   │   │   ├── cs.lng
    │   │   │   ├── de.lng
    │   │   │   ├── en.lng
    │   │   │   ├── es.lng
    │   │   │   ├── et.lng
    │   │   │   ├── fi.lng
    │   │   │   ├── he.lng
    │   │   │   ├── hi.lng
    │   │   │   ├── hu.lng
    │   │   │   ├── hy.lng
    │   │   │   ├── ja.lng
    │   │   │   ├── ka.lng
    │   │   │   ├── ko.lng
    │   │   │   ├── lt.lng
    │   │   │   ├── lv.lng
    │   │   │   ├── pl.lng
    │   │   │   ├── README.md
    │   │   │   ├── ru.lng
    │   │   │   ├── tr.lng
    │   │   │   ├── uk.lng
    │   │   │   └── zh.lng
    │   │   ├── lang_bidi_test.go
    │   │   ├── lang_consistency_test.go
    │   │   ├── lang_contamination_test.go
    │   │   ├── lang_fallback_priority_test.go
    │   │   ├── lang.go
    │   │   ├── lang_homoglyphs_test.go
    │   │   ├── lang_packs_test.go
    │   │   ├── lang_scripts_test.go
    │   │   ├── lang_test.go
    │   │   ├── language_list_test.go
    │   │   ├── languages.go
    │   │   ├── msg_keys_test.go
    │   │   ├── packs.go
    │   │   └── settings_translations_test.go
    │   ├── ini
    │   │   ├── ini.go
    │   │   └── ini_test.go
    │   ├── keymap
    │   │   ├── farkeys.go
    │   │   ├── hotkeys.go
    │   │   ├── input.go
    │   │   ├── input_translation_test.go
    │   │   ├── keymap_test.go
    │   │   ├── kitty.go
    │   │   ├── mackeys.go
    │   │   ├── mackeys_test.go
    │   │   ├── remap.go
    │   │   ├── terminal_mouse_offset_test.go
    │   │   ├── translate_kitty_test.go
    │   │   ├── ttyx.go
    │   │   └── ttyx_keys_test.go
    │   ├── luaplug
    │   │   ├── convert.go
    │   │   ├── convert_test.go
    │   │   ├── f4rpc.go
    │   │   ├── ffi.go
    │   │   ├── ffi_test.go
    │   │   ├── goid.go
    │   │   ├── luastate_test.go
    │   │   ├── runtime.go
    │   │   ├── runtime_test.go
    │   │   └── sandbox.go
    │   ├── macro
    │   │   ├── engine.go
    │   │   ├── export.go
    │   │   ├── export_test.go
    │   │   ├── lua_api.go
    │   │   ├── lua.go
    │   │   ├── lua_test.go
    │   │   ├── plugin_calls.go
    │   │   ├── plugin_calls_test.go
    │   │   ├── reload.go
    │   │   └── reload_test.go
    │   ├── media
    │   │   ├── application.go
    │   │   ├── audio_decode.go
    │   │   ├── audio_decode_test.go
    │   │   ├── audio.go
    │   │   ├── audio_oto.go
    │   │   ├── audio_stub.go
    │   │   ├── blit_test.go
    │   │   ├── image_bmp.go
    │   │   ├── image_console_stats.go
    │   │   ├── image_console_stats_test.go
    │   │   ├── image_decode.go
    │   │   ├── image_decode_test.go
    │   │   ├── image_external.go
    │   │   ├── image_external_test.go
    │   │   ├── image_formats_test.go
    │   │   ├── image_gallery.go
    │   │   ├── image_gallery_test.go
    │   │   ├── image.go
    │   │   ├── image_native_darwin.go
    │   │   ├── image_native_darwin_test.go
    │   │   ├── image_preview.go
    │   │   ├── image_preview_test.go
    │   │   ├── image_qoi.go
    │   │   ├── image_slideshow.go
    │   │   ├── image_slideshow_test.go
    │   │   ├── image_test.go
    │   │   ├── image_transform.go
    │   │   ├── image_transform_test.go
    │   │   ├── image_view.go
    │   │   ├── image_view_orient_test.go
    │   │   ├── image_view_overlay_test.go
    │   │   ├── image_view_test.go
    │   │   ├── overlay_console.go
    │   │   ├── overlay_console_key_test.go
    │   │   ├── overlay_x11.go
    │   │   ├── overlay_x11_test.go
    │   │   ├── tools.go
    │   │   ├── tools_test.go
    │   │   ├── video.go
    │   │   ├── video_test.go
    │   │   ├── video_view_coverage_test.go
    │   │   └── video_view.go
    │   ├── netproxy
    │   │   ├── coverage_test.go
    │   │   ├── keepalive_linux_test.go
    │   │   ├── netproxy.go
    │   │   └── netproxy_test.go
    │   ├── numeric
    │   │   ├── memory.go
    │   │   ├── numeric.go
    │   │   ├── numeric_test.go
    │   │   └── size.go
    │   ├── panel
    │   │   ├── actions.go
    │   │   ├── apply.go
    │   │   ├── apply_shutdown.go
    │   │   ├── associations_editor.go
    │   │   ├── associations.go
    │   │   ├── associations_ui.go
    │   │   ├── audio_decode_panel_test.go
    │   │   ├── background_jobs_session_test.go
    │   │   ├── bookmarks_dialog.go
    │   │   ├── bookmarks_dialog_test.go
    │   │   ├── bookmarks.go
    │   │   ├── bridge_texteditor.go
    │   │   ├── bridge_visren_coverage_test.go
    │   │   ├── bridge_visren.go
    │   │   ├── cmd_session_test.go
    │   │   ├── console.go
    │   │   ├── context_editors_test.go
    │   │   ├── coverage_player_helpers_test.go
    │   │   ├── drives_bookmarks.go
    │   │   ├── drives_bookmarks_ui.go
    │   │   ├── drives_menu.go
    │   │   ├── drives_menu_unix.go
    │   │   ├── drives_menu_windows.go
    │   │   ├── edit_command_test.go
    │   │   ├── exec.go
    │   │   ├── file_panel_test.go
    │   │   ├── frame_coverage_test.go
    │   │   ├── frame_dragdrop.go
    │   │   ├── frame_externalui.go
    │   │   ├── frame.go
    │   │   ├── frame_manager_test_helpers_test.go
    │   │   ├── frame_procenv.go
    │   │   ├── frame_semantic.go
    │   │   ├── frame_translator.go
    │   │   ├── frame_workspace.go
    │   │   ├── frame_workspace_terminal.go
    │   │   ├── fuse_list_coverage_extra_test.go
    │   │   ├── fuse_list_coverage_test.go
    │   │   ├── fuse_list.go
    │   │   ├── fuse_mount_coverage_test.go
    │   │   ├── fuse_mount.go
    │   │   ├── hints.go
    │   │   ├── host.go
    │   │   ├── host_input_modes.go
    │   │   ├── host_input_modes_other.go
    │   │   ├── host_input_modes_test.go
    │   │   ├── host_input_modes_windows.go
    │   │   ├── hotkey_conditions.go
    │   │   ├── info.go
    │   │   ├── info_panel_test.go
    │   │   ├── info_usage.go
    │   │   ├── issue863_terminal_test.go
    │   │   ├── issue915_keybar_test.go
    │   │   ├── list.go
    │   │   ├── list_reconnect.go
    │   │   ├── list_size.go
    │   │   ├── list_size_test.go
    │   │   ├── lookup.go
    │   │   ├── main_test.go
    │   │   ├── menubar_dropdown_test.go
    │   │   ├── mock_pty_test.go
    │   │   ├── panels_frame_pty_test.go
    │   │   ├── panels_frame_test.go
    │   │   ├── pins.go
    │   │   ├── player.go
    │   │   ├── player_test.go
    │   │   ├── plugin_hotkeys.go
    │   │   ├── plugins.go
    │   │   ├── prefixes.go
    │   │   ├── prefixes_registry_windows_test.go
    │   │   ├── prefixes_test.go
    │   │   ├── press_key_test.go
    │   │   ├── process_environment_panel_test.go
    │   │   ├── prompt.go
    │   │   ├── quickview_colors_test.go
    │   │   ├── quickview.go
    │   │   ├── quick_view_panel_test.go
    │   │   ├── quick_view_provider_test.go
    │   │   ├── reconnect_test.go
    │   │   ├── remote.go
    │   │   ├── selection_panel_test.go
    │   │   ├── session_cmd.go
    │   │   ├── settings.go
    │   │   ├── shell_session_test.go
    │   │   ├── sort.go
    │   │   ├── sort_groups_test.go
    │   │   ├── state.go
    │   │   ├── temp_coverage_test.go
    │   │   ├── temp.go
    │   │   ├── uri_navigation_test.go
    │   │   ├── usermenu_coverage_test.go
    │   │   ├── usermenu_farfile.go
    │   │   ├── usermenu.go
    │   │   ├── usermenu_ini.go
    │   │   ├── usermenu_script_coverage_test.go
    │   │   ├── usermenu_script.go
    │   │   ├── usermenu_subst.go
    │   │   ├── usermenu_ui.go
    │   │   ├── user_menu_ui_test.go
    │   │   ├── workspace.go
    │   │   └── workspace_startup_test.go
    │   ├── paneltest
    │   │   ├── doc.go
    │   │   ├── frame.go
    │   │   └── mock_pty.go
    │   ├── piecetable
    │   │   ├── concurrent_test.go
    │   │   ├── lineindex_equivalence_test.go
    │   │   ├── lineindex.go
    │   │   ├── lineindex_test.go
    │   │   ├── piecetable.go
    │   │   └── piecetable_test.go
    │   ├── plughost
    │   │   ├── application.go
    │   │   ├── contributions.go
    │   │   ├── extui_coverage_test.go
    │   │   ├── extui.go
    │   │   ├── extui_test.go
    │   │   ├── ffi.go
    │   │   ├── ffi_test.go
    │   │   ├── host_coverage_test.go
    │   │   ├── host.go
    │   │   ├── identity_test.go
    │   │   ├── lifecycle_coverage_test.go
    │   │   ├── manager_coverage_test.go
    │   │   ├── manager.go
    │   │   ├── manager_lifecycle_test.go
    │   │   ├── manager_names_test.go
    │   │   ├── menu_items.go
    │   │   ├── panel_providers.go
    │   │   ├── permissions.go
    │   │   ├── permissions_test.go
    │   │   ├── permissions_ui.go
    │   │   ├── permissions_ui_test.go
    │   │   ├── plugring.go
    │   │   ├── plugring_meta.go
    │   │   ├── plugring_meta_test.go
    │   │   ├── rpc_commands.go
    │   │   ├── rpc_panel_coverage_test.go
    │   │   ├── rpc_panel.go
    │   │   ├── rpc_vfs_coverage_test.go
    │   │   ├── rpc_vfs.go
    │   │   ├── rpc_vfs_test.go
    │   │   ├── scaffold.go
    │   │   ├── scaffold_test.go
    │   │   ├── settings_permissions_test.go
    │   │   ├── transport_lua.go
    │   │   ├── transport_lua_rpc_test.go
    │   │   ├── transport_lua_test.go
    │   │   ├── transport_rpc_coverage_test.go
    │   │   ├── transport_rpc.go
    │   │   ├── transport_rpc_test.go
    │   │   ├── transport_wazero.go
    │   │   └── transport_wazero_test.go
    │   ├── semantic
    │   │   ├── fields.go
    │   │   └── fields_test.go
    │   ├── settings
    │   │   ├── benchmark_test.go
    │   │   ├── catalog.go
    │   │   ├── center.go
    │   │   ├── center_test.go
    │   │   ├── choice_help.go
    │   │   ├── choice_help_test.go
    │   │   ├── choice_help.tsv
    │   │   ├── chord.go
    │   │   ├── collection_palette_test.go
    │   │   ├── collection_ui.go
    │   │   ├── core.go
    │   │   ├── edit.go
    │   │   ├── edit_test.go
    │   │   ├── enter_test.go
    │   │   ├── extra.go
    │   │   ├── help.go
    │   │   ├── host.go
    │   │   ├── hotkeys_persistence_test.go
    │   │   ├── inventory_test.go
    │   │   ├── main_test.go
    │   │   ├── manual_save_test.go
    │   │   ├── mouse_test.go
    │   │   ├── operations.go
    │   │   ├── plugin_catalog.go
    │   │   ├── plugins.go
    │   │   ├── providers.go
    │   │   ├── radios.go
    │   │   ├── radios_test.go
    │   │   ├── records.go
    │   │   ├── russian_test.go
    │   │   ├── scoped_test.go
    │   │   ├── search_layout_test.go
    │   │   ├── startup_test.go
    │   │   ├── trace.go
    │   │   ├── trace_test.go
    │   │   ├── transactions_test.go
    │   │   ├── user_menu.go
    │   │   └── user_menu_test.go
    │   ├── settingstest
    │   │   └── russian.go
    │   ├── sheet
    │   │   ├── cell.go
    │   │   ├── expr.go
    │   │   ├── sheet.go
    │   │   ├── sheet_test.go
    │   │   ├── store.go
    │   │   └── xlsx.go
    │   ├── sysinfo
    │   │   ├── cpu_darwin.go
    │   │   ├── cpu.go
    │   │   ├── cpu_linux.go
    │   │   ├── cpu_other.go
    │   │   ├── cpu_windows.go
    │   │   ├── drives.go
    │   │   ├── drives_unix.go
    │   │   ├── drives_windows.go
    │   │   ├── fs_darwin.go
    │   │   ├── fs.go
    │   │   ├── fs_linux.go
    │   │   ├── fs_other.go
    │   │   ├── fs_windows.go
    │   │   ├── fs_windows_test.go
    │   │   ├── gpu_darwin.go
    │   │   ├── gpu.go
    │   │   ├── gpu_linux.go
    │   │   ├── gpu_other.go
    │   │   ├── gpu_windows.go
    │   │   ├── mem.go
    │   │   ├── mem_linux.go
    │   │   ├── mem_other.go
    │   │   └── mem_windows.go
    │   ├── terminal
    │   │   ├── ansi.go
    │   │   ├── ansi_sync_test.go
    │   │   ├── ansi_test.go
    │   │   ├── application.go
    │   │   ├── attr.go
    │   │   ├── backend.go
    │   │   ├── child_env.go
    │   │   ├── child_process.go
    │   │   ├── clipboard_async.go
    │   │   ├── clipboard.go
    │   │   ├── clipboard_test.go
    │   │   ├── console_buffer_other.go
    │   │   ├── console_buffer_windows.go
    │   │   ├── console_host_windows.go
    │   │   ├── console_overlay_other.go
    │   │   ├── console_overlay_windows.go
    │   │   ├── console_overlay_windows_test.go
    │   │   ├── far2l_auth.go
    │   │   ├── far2l_auth_test.go
    │   │   ├── far2l_image.go
    │   │   ├── far2l_image_test.go
    │   │   ├── graphics_compat.go
    │   │   ├── graphics_compat_test.go
    │   │   ├── graphics_probe.go
    │   │   ├── graphics_probe_test.go
    │   │   ├── graphics_probe_windows.go
    │   │   ├── jobs.go
    │   │   ├── jobs_test.go
    │   │   ├── kitty_coverage_test.go
    │   │   ├── kitty.go
    │   │   ├── kitty_metrics_test.go
    │   │   ├── kitty_placements.go
    │   │   ├── kitty_placements_test.go
    │   │   ├── kitty_test.go
    │   │   ├── log_console_other.go
    │   │   ├── log_console_windows.go
    │   │   ├── log_vfs.go
    │   │   ├── log_vfs_test.go
    │   │   ├── main_test.go
    │   │   ├── overlay.go
    │   │   ├── pe_subsystem.go
    │   │   ├── pe_subsystem_test.go
    │   │   ├── process_environment.go
    │   │   ├── process_environment_runtime_unix.go
    │   │   ├── process_environment_runtime_windows.go
    │   │   ├── process_environment_test.go
    │   │   ├── pty_bsd_dragonfly.go
    │   │   ├── pty_bsd_freebsd.go
    │   │   ├── pty_bsd.go
    │   │   ├── pty_bsd_test.go
    │   │   ├── pty_cloexec_test.go
    │   │   ├── pty_darwin.go
    │   │   ├── pty_diag_unix.go
    │   │   ├── pty_diag_unix_test.go
    │   │   ├── pty_diag_windows.go
    │   │   ├── pty.go
    │   │   ├── pty_linux.go
    │   │   ├── pty_pollable_test.go
    │   │   ├── pty_ptm.go
    │   │   ├── pty_ptm_netbsd.go
    │   │   ├── pty_ptm_openbsd.go
    │   │   ├── pty_solaris.go
    │   │   ├── pty_test.go
    │   │   ├── pty_windows.go
    │   │   ├── pty_windows_test.go
    │   │   ├── redraw.go
    │   │   ├── runner.go
    │   │   ├── runner_test.go
    │   │   ├── runner_unix.go
    │   │   ├── runner_unix_test.go
    │   │   ├── runner_windows.go
    │   │   ├── runner_windows_test.go
    │   │   ├── selection_test.go
    │   │   ├── session_attach_identity_test.go
    │   │   ├── session_attach_payload_test.go
    │   │   ├── session_daemon_test.go
    │   │   ├── session_unix_coverage_test.go
    │   │   ├── session_unix.go
    │   │   ├── session_unix_test.go
    │   │   ├── session_windows.go
    │   │   ├── shellmode.go
    │   │   ├── shellmode_test.go
    │   │   ├── sixel.go
    │   │   ├── sixel_layers_test.go
    │   │   ├── sixel_terminal.go
    │   │   ├── sixel_terminal_test.go
    │   │   ├── sixel_test.go
    │   │   ├── solaris_pty_alloc_test.go
    │   │   ├── solaris_pty_backend_test.go
    │   │   ├── solaris_pty.go
    │   │   ├── solaris_streams.go
    │   │   ├── solaris_streams_mock_linux_test.go
    │   │   ├── solaris_streams_mock_other_test.go
    │   │   ├── solaris_streams_mock_test.go
    │   │   ├── solaris_streams_test.go
    │   │   ├── ttyx_probe_coverage_unix_test.go
    │   │   ├── ttyx_probe.go
    │   │   ├── ttyx_probe_parse.go
    │   │   ├── ttyx_probe_test.go
    │   │   ├── ttyx_probe_unix.go
    │   │   ├── ttyx_probe_windows.go
    │   │   ├── ttyx_session.go
    │   │   ├── view.go
    │   │   ├── view_semantic.go
    │   │   ├── view_semantic_test.go
    │   │   ├── view_test.go
    │   │   ├── wineprobe.go
    │   │   ├── wineprobe_other.go
    │   │   ├── wineprobe_test.go
    │   │   ├── wineprobe_windows.go
    │   │   └── zzz_pty_leak_check_test.go
    │   ├── testutil
    │   │   ├── dialog.go
    │   │   ├── doc.go
    │   │   ├── frame.go
    │   │   ├── frame_test.go
    │   │   ├── input.go
    │   │   ├── main.go
    │   │   ├── numeric.go
    │   │   ├── paths_coverage_test.go
    │   │   ├── paths.go
    │   │   ├── race_disabled.go
    │   │   ├── race_enabled.go
    │   │   └── rpc.go
    │   ├── textlayout
    │   │   ├── cluster.go
    │   │   ├── wrap.go
    │   │   └── wrap_test.go
    │   ├── textsearch
    │   │   └── search.go
    │   ├── theme
    │   │   ├── colors_cursor_test.go
    │   │   ├── colors.go
    │   │   ├── colorspace.go
    │   │   ├── colorspace_test.go
    │   │   ├── colors_test.go
    │   │   ├── farcolor.go
    │   │   ├── farcolor_test.go
    │   │   ├── highlight_files_test.go
    │   │   ├── highlight.go
    │   │   ├── style_combo_colors_test.go
    │   │   ├── style_completeness_test.go
    │   │   ├── style_custom_test.go
    │   │   ├── style_default_dark_test.go
    │   │   ├── style.go
    │   │   ├── style_indicator_test.go
    │   │   ├── style_overrides_test.go
    │   │   ├── styles
    │   │   │   ├── classic.ini
    │   │   │   ├── default_dark.ini
    │   │   │   ├── modern.ini
    │   │   │   ├── radiola.ini
    │   │   │   └── radiola.md
    │   │   ├── style_test.go
    │   │   └── table.go
    │   ├── toast
    │   │   └── toast.go
    │   ├── ttyx
    │   │   ├── coverage_edges_test.go
    │   │   ├── coverage_state_test.go
    │   │   ├── keys_coverage_test.go
    │   │   ├── keys.go
    │   │   ├── openfor_test.go
    │   │   ├── overlay.go
    │   │   ├── overlay_lifecycle_test.go
    │   │   ├── session.go
    │   │   ├── session_state_test.go
    │   │   ├── state_coverage_test.go
    │   │   ├── ttyx_events_test.go
    │   │   ├── ttyx_test.go
    │   │   ├── watch_coverage_test.go
    │   │   └── watch.go
    │   ├── unpack
    │   │   ├── coverage_test.go
    │   │   ├── unpack.go
    │   │   └── unpack_test.go
    │   ├── update
    │   │   ├── assets_test.go
    │   │   ├── cli.go
    │   │   ├── cli_test.go
    │   │   ├── coverage_test.go
    │   │   ├── elevation_manual_windows_test.go
    │   │   ├── elevation_other.go
    │   │   ├── elevation_windows.go
    │   │   ├── helper_args.go
    │   │   ├── libc_default.go
    │   │   ├── libc_default_test.go
    │   │   ├── libc_musl.go
    │   │   ├── libc_musl_test.go
    │   │   ├── selfexec.go
    │   │   ├── selfexec_linux.go
    │   │   ├── selfexec_linux_test.go
    │   │   ├── selfexec_other.go
    │   │   ├── selfexec_termux.go
    │   │   ├── selfexec_test.go
    │   │   ├── update.go
    │   │   └── update_test.go
    │   ├── viewer
    │   │   ├── application.go
    │   │   ├── backend.go
    │   │   ├── backend_test.go
    │   │   ├── binary.go
    │   │   ├── colorizer.go
    │   │   ├── disasm.go
    │   │   ├── disasm_test.go
    │   │   ├── highlight.go
    │   │   ├── highlight_test.go
    │   │   ├── links.go
    │   │   ├── links_test.go
    │   │   ├── main_test.go
    │   │   ├── menubar_test.go
    │   │   ├── search.go
    │   │   ├── tail_test.go
    │   │   ├── text.go
    │   │   ├── text_test.go
    │   │   ├── title.go
    │   │   ├── topbar.go
    │   │   ├── topbar_test.go
    │   │   ├── view.go
    │   │   ├── view_semantic.go
    │   │   ├── view_test.go
    │   │   ├── wordnav.go
    │   │   └── wordnav_test.go
    │   ├── vtvibe
    │   │   ├── ap.go
    │   │   ├── ap_test.go
    │   │   ├── coverage_test.go
    │   │   ├── defaults.go
    │   │   ├── memtree.go
    │   │   ├── pack.go
    │   │   ├── pack_test.go
    │   │   ├── provider.go
    │   │   ├── provider_test.go
    │   │   ├── session.go
    │   │   ├── session_test.go
    │   │   ├── vfs_coverage_test.go
    │   │   └── vfs.go
    │   └── wincon
    │       ├── blit_test.go
    │       ├── coverage_test.go
    │       ├── geometry.go
    │       ├── layered.go
    │       ├── layered_test.go
    │       ├── overlay_other.go
    │       ├── overlay_state.go
    │       ├── overlay_state_test.go
    │       ├── overlay_windows.go
    │       ├── overlay_windows_test.go
    │       ├── stats.go
    │       ├── stats_windows.go
    │       └── wincon_test.go
    ├── LICENSE
    ├── .mcp.json
    ├── packaging
    │   ├── linux
    │   │   └── f4.desktop
    │   └── macos
    │       └── Info.plist
    ├── plugins
    │   ├── android
    │   │   ├── adb_integration_test.go
    │   │   ├── adb_sync.go
    │   │   ├── adb_sync_test.go
    │   │   ├── adb_transport.go
    │   │   ├── adb_transport_test.go
    │   │   ├── command_runner_info_test.go
    │   │   ├── device.go
    │   │   ├── device_test.go
    │   │   ├── fish_pool.go
    │   │   ├── fish_pool_test.go
    │   │   ├── info.go
    │   │   ├── info_test.go
    │   │   ├── manager.go
    │   │   ├── manager_test.go
    │   │   ├── pathutil.go
    │   │   ├── pathutil_test.go
    │   │   ├── README.md
    │   │   ├── sync_vfs.go
    │   │   └── sync_vfs_test.go
    │   ├── archive
    │   │   ├── archive.go
    │   │   ├── archive_materialize_unix_test.go
    │   │   ├── archive_plugin_test.go
    │   │   ├── archive_test.go
    │   │   ├── archive_write_regression_test.go
    │   │   ├── clone_test.go
    │   │   ├── compressed_regular_test.go
    │   │   ├── extraction_security_test.go
    │   │   ├── issue1179_sfx_test.go
    │   │   ├── issue1179_volumes_test.go
    │   │   ├── issue815_f3_test.go
    │   │   ├── issue816_multivolume_test.go
    │   │   ├── issue816_password_retry_test.go
    │   │   ├── issue915_total_progress_test.go
    │   │   ├── materialize.go
    │   │   ├── multivolume_rar.go
    │   │   ├── password_coverage_test.go
    │   │   ├── password.go
    │   │   ├── password_test.go
    │   │   ├── production_regression_test.go
    │   │   ├── provider.go
    │   │   ├── provider_special_unix_test.go
    │   │   ├── provider_test.go
    │   │   ├── repro_test.go
    │   │   ├── sfx.go
    │   │   ├── sfx_test.go
    │   │   ├── vfs.go
    │   │   ├── vfs_nested_test.go
    │   │   ├── vfs_test.go
    │   │   ├── zipcrypto_checkbyte_test.go
    │   │   ├── zip_encoding.go
    │   │   └── zip_encoding_test.go
    │   ├── chroma
    │   │   ├── chroma.go
    │   │   └── chroma_test.go
    │   ├── cloudfox
    │   │   ├── cloud_vfs.go
    │   │   ├── cloud_vfs_share_test.go
    │   │   ├── cloud_vfs_test.go
    │   │   ├── context_editor_test.go
    │   │   ├── credential_scope.go
    │   │   ├── credential_scope_test.go
    │   │   ├── dialog.go
    │   │   ├── dialog_google_test.go
    │   │   ├── dialog_s3_test.go
    │   │   ├── manager.go
    │   │   ├── oauth.go
    │   │   ├── password_prompt.go
    │   │   ├── password_prompt_test.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── provider_capabilities_test.go
    │   │   ├── provider_google.go
    │   │   ├── provider_google_production_test.go
    │   │   ├── provider_google_real_native_integration_test.go
    │   │   ├── provider_google_share.go
    │   │   ├── provider_google_share_test.go
    │   │   ├── provider_google_test.go
    │   │   ├── provider_helpers.go
    │   │   ├── provider_helpers_test.go
    │   │   ├── provider_mutation_test.go
    │   │   ├── provider_real_diagnostics_test.go
    │   │   ├── provider_real_saved_integration_test.go
    │   │   ├── provider_real_semantics_test.go
    │   │   ├── provider_real_sharing_integration_test.go
    │   │   ├── provider_real_upload_cancellation_test.go
    │   │   ├── provider_s3_discovery_core_test.go
    │   │   ├── provider_s3_discovery_regression_test.go
    │   │   ├── provider_s3.go
    │   │   ├── provider_s3_real_discovery_test.go
    │   │   ├── provider_s3_share.go
    │   │   ├── provider_s3_share_test.go
    │   │   ├── provider_s3_test.go
    │   │   ├── provider_webdav_edge_test.go
    │   │   ├── provider_webdav.go
    │   │   ├── provider_webdav_integration_test.go
    │   │   ├── provider_webdav_share.go
    │   │   ├── provider_webdav_share_test.go
    │   │   ├── provider_webdav_test.go
    │   │   ├── provider_yandex_cache.go
    │   │   ├── provider_yandex.go
    │   │   ├── provider_yandex_info.go
    │   │   ├── provider_yandex_production_test.go
    │   │   ├── provider_yandex_share.go
    │   │   ├── provider_yandex_share_test.go
    │   │   ├── provider_yandex_test.go
    │   │   ├── secrets.go
    │   │   ├── secrets_test.go
    │   │   ├── session.go
    │   │   ├── settings_center.go
    │   │   ├── settings_center_test.go
    │   │   ├── settings_russian_test.go
    │   │   ├── store.go
    │   │   ├── store_lock_unix.go
    │   │   ├── store_lock_windows.go
    │   │   ├── store_test.go
    │   │   ├── test_main_test.go
    │   │   ├── types.go
    │   │   ├── uri.go
    │   │   ├── uri_test.go
    │   │   ├── vault.go
    │   │   └── yandex_code_prompt.go
    │   ├── dummy_internal
    │   │   ├── dummy_internal.go
    │   │   └── dummy_internal_test.go
    │   ├── dummy_lua
    │   │   ├── plugin.lua
    │   │   └── README.md
    │   ├── dummy_rpc
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── envman
    │   │   ├── codec.go
    │   │   ├── codec_test.go
    │   │   ├── commands.go
    │   │   ├── commands_test.go
    │   │   ├── dialogs.go
    │   │   ├── environment_document.go
    │   │   ├── far3_import.go
    │   │   ├── far3_import_other.go
    │   │   ├── far3_import_test.go
    │   │   ├── far3_import_ui.go
    │   │   ├── far3_import_windows.go
    │   │   ├── far3_import_windows_test.go
    │   │   ├── manager_frame.go
    │   │   ├── manager_ops.go
    │   │   ├── manager_ui.go
    │   │   ├── messages.go
    │   │   ├── model.go
    │   │   ├── model_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── README.md
    │   │   ├── settings_center.go
    │   │   ├── settings.go
    │   │   ├── settings_russian_test.go
    │   │   ├── settings_test.go
    │   │   ├── strings.go
    │   │   ├── ui_test.go
    │   │   └── vfs_io.go
    │   ├── id3editor
    │   │   ├── plugin_contributions_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_handle_test.go
    │   │   ├── plugin_paths_test.go
    │   │   └── plugin_test.go
    │   ├── ios
    │   │   ├── afc_vfs.go
    │   │   ├── afc_vfs_test.go
    │   │   ├── apps.go
    │   │   ├── core_access.go
    │   │   ├── core_access_stub.go
    │   │   ├── core_access_supported.go
    │   │   ├── core_access_supported_test.go
    │   │   ├── core_tunnel_supported.go
    │   │   ├── core_vfs.go
    │   │   ├── core_vfs_test.go
    │   │   ├── coverage_edges_test.go
    │   │   ├── coverage_plugins_test.go
    │   │   ├── coverage_test.go
    │   │   ├── internal
    │   │   │   ├── afcproto
    │   │   │   │   ├── client.go
    │   │   │   │   ├── client_test.go
    │   │   │   │   ├── doc.go
    │   │   │   │   ├── errors.go
    │   │   │   │   ├── file.go
    │   │   │   │   ├── path.go
    │   │   │   │   ├── protocol.go
    │   │   │   │   ├── protocol_test.go
    │   │   │   │   └── types.go
    │   │   │   └── corefileservice
    │   │   │       ├── doc.go
    │   │   │       ├── fileservice.go
    │   │   │       └── fileservice_test.go
    │   │   ├── ios_integration_test.go
    │   │   ├── LICENSE.go-ios
    │   │   ├── manager.go
    │   │   ├── manager_test.go
    │   │   ├── native_source.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── README.md
    │   │   ├── selectors.go
    │   │   ├── selectors_test.go
    │   │   ├── services.go
    │   │   └── services_test.go
    │   ├── mediainfo
    │   │   ├── analyzer.go
    │   │   ├── backend_test.go
    │   │   ├── cache.go
    │   │   ├── cache_test.go
    │   │   ├── config_dialog.go
    │   │   ├── config_dialog_test.go
    │   │   ├── coverage_test.go
    │   │   ├── dialog.go
    │   │   ├── dialog_theme_test.go
    │   │   ├── exif_report_test.go
    │   │   ├── format_gaps_test.go
    │   │   ├── locale.go
    │   │   ├── macro.go
    │   │   ├── macro_test.go
    │   │   ├── matroska_bounds_test.go
    │   │   ├── model.go
    │   │   ├── open.go
    │   │   ├── open_test.go
    │   │   ├── parse_audio.go
    │   │   ├── parse_ebu_stl.go
    │   │   ├── parse_heif.go
    │   │   ├── parse_image.go
    │   │   ├── parse_iso_coverage_test.go
    │   │   ├── parse_iso.go
    │   │   ├── parse_matroska.go
    │   │   ├── parse_riff.go
    │   │   ├── parse_subtitle.go
    │   │   ├── parse_tiff.go
    │   │   ├── parse_tiff_limits_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── quickview_provider.go
    │   │   ├── quickview_provider_test.go
    │   │   ├── README.md
    │   │   ├── render.go
    │   │   ├── render_limits_test.go
    │   │   ├── report_text.go
    │   │   ├── report_view.go
    │   │   ├── settings_center.go
    │   │   ├── settings.go
    │   │   ├── settings_russian_test.go
    │   │   ├── settings_test.go
    │   │   ├── source.go
    │   │   ├── subtitle_limits_test.go
    │   │   └── util.go
    │   ├── netfox
    │   │   ├── context_editor_test.go
    │   │   ├── coverage_test.go
    │   │   ├── crypto.go
    │   │   ├── crypto_test.go
    │   │   ├── dev
    │   │   │   ├── README.md
    │   │   │   └── unxed_f4_issue_316.json
    │   │   ├── dialog.go
    │   │   ├── dialog_test.go
    │   │   ├── fish_clone_session_test.go
    │   │   ├── fish_dialer_test.go
    │   │   ├── fishplus
    │   │   │   ├── cancel_test.go
    │   │   │   ├── cand
    │   │   │   ├── exec.go
    │   │   │   ├── exec_test.go
    │   │   │   ├── fs.go
    │   │   │   ├── fs_test.go
    │   │   │   ├── hash.go
    │   │   │   ├── hash_test.go
    │   │   │   ├── helper.ps1
    │   │   │   ├── helper.sh
    │   │   │   ├── job.go
    │   │   │   ├── job_test.go
    │   │   │   ├── keepalive.go
    │   │   │   ├── keepalive_test.go
    │   │   │   ├── ls.go
    │   │   │   ├── ls_test.go
    │   │   │   ├── mutate.go
    │   │   │   ├── mutate_test.go
    │   │   │   ├── patch.go
    │   │   │   ├── patch_test.go
    │   │   │   ├── paths.go
    │   │   │   ├── paths_test.go
    │   │   │   ├── random_fixture_test.go
    │   │   │   ├── read.go
    │   │   │   ├── read_test.go
    │   │   │   ├── script.go
    │   │   │   ├── script_pwsh_test.go
    │   │   │   ├── script_test.go
    │   │   │   ├── search.go
    │   │   │   ├── search_test.go
    │   │   │   ├── session.go
    │   │   │   ├── session_pwsh_test.go
    │   │   │   ├── session_test.go
    │   │   │   ├── sizes
    │   │   │   ├── WINDOWS_PORT.md
    │   │   │   ├── write.go
    │   │   │   └── write_test.go
    │   │   ├── fish_pool.go
    │   │   ├── fish_reconnect_entry_test.go
    │   │   ├── fish_reconnect_test.go
    │   │   ├── fish_vfs.go
    │   │   ├── fish_vfs_test.go
    │   │   ├── ftp_clone_test.go
    │   │   ├── ftp_vfs.go
    │   │   ├── lang_test.go
    │   │   ├── netfox.go
    │   │   ├── netfox_test.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── proxy_dialog.go
    │   │   ├── registry.go
    │   │   ├── settings_center.go
    │   │   ├── settings_center_test.go
    │   │   ├── settings_russian_test.go
    │   │   ├── sftp_command_test.go
    │   │   ├── sftp_dial_test.go
    │   │   ├── sftp_uri.go
    │   │   ├── sftp_vfs.go
    │   │   ├── ssh_agent_forwarding_test.go
    │   │   ├── ssh_agent_unix.go
    │   │   ├── ssh_agent_windows.go
    │   │   ├── ssh_agent_windows_test.go
    │   │   ├── ssh_dial.go
    │   │   ├── ssh_dial_test.go
    │   │   ├── ssh_keepalive_test.go
    │   │   ├── ssh_known_hosts.go
    │   │   ├── ssh_known_hosts_test.go
    │   │   ├── ssh_pty.go
    │   │   ├── vfs_abs_test.go
    │   │   └── vfs.go
    │   ├── sqlite
    │   │   ├── coverage_test.go
    │   │   ├── locale.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── ui.go
    │   │   └── ui_test.go
    │   └── visren
    │       ├── config.go
    │       ├── config_test.go
    │       ├── coverage_test.go
    │       ├── dialog.go
    │       ├── dialog_test.go
    │       ├── editor.go
    │       ├── editor_test.go
    │       ├── engine_test.go
    │       ├── LICENSE.upstream
    │       ├── masks.go
    │       ├── metadata.go
    │       ├── metadata_test.go
    │       ├── model.go
    │       ├── plugin.go
    │       ├── plugin_test.go
    │       ├── rename.go
    │       ├── rename_test.go
    │       ├── replace.go
    │       ├── settings_center.go
    │       ├── settings_center_test.go
    │       ├── settings_russian_test.go
    │       ├── transforms.go
    │       └── word_div_prompt_test.go
    ├── plugring
    │   ├── hello_plugring.lua
    │   └── index.yaml
    ├── README.md
    ├── scripts
    │   ├── filelist_update.sh
    │   ├── test_plugins.sh
    │   └── test_resurrect.sh
    ├── sdk
    │   ├── extui
    │   │   ├── model_coverage_test.go
    │   │   ├── model.go
    │   │   └── model_test.go
    │   ├── f4plugin
    │   │   ├── plugin.go
    │   │   └── plugin_test.go
    │   ├── f4rpc
    │   │   ├── mux.go
    │   │   └── mux_test.go
    │   ├── f4settings
    │   │   ├── localization_test.go
    │   │   ├── settings.go
    │   │   ├── settings_test.go
    │   │   └── struct_provider.go
    │   └── lua
    │       └── f4rpc.lua
    ├── skills-lock.json
    ├── tools
    │   ├── conpty_probe_child.py
    │   ├── conpty_probe.py
    │   ├── conptyreconcile
    │   │   ├── capture.go
    │   │   ├── clear_probe_windows.go
    │   │   ├── command_compare_windows.go
    │   │   ├── command_probe_windows.go
    │   │   ├── command_suite_windows.go
    │   │   ├── control_stream.go
    │   │   ├── control_stream_test.go
    │   │   ├── edge_probe_windows.go
    │   │   ├── emitter.go
    │   │   ├── empty_probe_windows.go
    │   │   ├── gate.go
    │   │   ├── gate_nonwindows.go
    │   │   ├── gate_windows.go
    │   │   ├── go.mod
    │   │   ├── go.sum
    │   │   ├── hash.go
    │   │   ├── host_constants.go
    │   │   ├── host_history.go
    │   │   ├── host_history_test.go
    │   │   ├── host_stream_chunking.go
    │   │   ├── host_stream_chunking_test.go
    │   │   ├── host_stream.go
    │   │   ├── host_stream_test.go
    │   │   ├── lifecycle_probe_windows.go
    │   │   ├── line_diff.go
    │   │   ├── logical_lines.go
    │   │   ├── logical_lines_test.go
    │   │   ├── main.go
    │   │   ├── native_probe.go
    │   │   ├── native_probe_nonwindows.go
    │   │   ├── native_probe_windows.go
    │   │   ├── payload_assertions.go
    │   │   ├── payload_assertions_test.go
    │   │   ├── pinned_host.go
    │   │   ├── pinned_host_nonwindows.go
    │   │   ├── pinned_host_windows.go
    │   │   ├── probe.go
    │   │   ├── quirk_probe_windows.go
    │   │   ├── reflow_probe.go
    │   │   ├── reflow_probe_windows.go
    │   │   ├── scrollback.go
    │   │   ├── scrollback_test.go
    │   │   ├── scroll_probe_windows.go
    │   │   ├── seeds.go
    │   │   ├── semantic_probe.go
    │   │   └── semantic_probe_windows.go
    │   ├── f4imgprobe
    │   │   ├── main.go
    │   │   └── README.txt
    │   ├── find_hardcoded.go
    │   ├── fishplus_probe.sh
    │   ├── fishplus_testlab
    │   │   ├── fishclient.py
    │   │   ├── TESTLAB.md
    │   │   └── test_patch.py
    │   ├── hardcode
    │   │   ├── hardcode.go
    │   │   └── hardcode_test.go
    │   ├── hardcoded_baseline.txt
    │   ├── icons
    │   │   ├── go.mod
    │   │   ├── go.sum
    │   │   ├── main.go
    │   │   ├── main_test.go
    │   │   └── third_party
    │   │       └── oksvg
    │   │           ├── definitions.go
    │   │           ├── draw.go
    │   │           ├── go.mod
    │   │           ├── icon_cursor.go
    │   │           ├── LICENSE
    │   │           ├── path_cursor.go
    │   │           ├── path_style.go
    │   │           ├── public.go
    │   │           ├── README.md
    │   │           ├── svg_icon.go
    │   │           ├── svg_path.go
    │   │           └── utils.go
    │   ├── langfmt
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── sanitize_native_probe_report.ps1
    │   ├── test_runner.sh
    │   ├── ttytest
    │   │   ├── analyze_log.py
    │   │   ├── README.md
    │   │   ├── scenarios.py
    │   │   └── ttytest.py
    │   ├── verify_native_probe_artifacts.ps1
    │   ├── vtui-screen
    │   │   ├── main.go
    │   │   ├── main_test.go
    │   │   └── README.md
    │   ├── wine_color_probe
    │   │   ├── main.go
    │   │   └── main_other.go
    │   └── wine_syscall_probe
    │       ├── go.mod
    │       ├── main.go
    │       └── probe_amd64.s
    └── vfs
        ├── bulk_copy_test.go
        ├── codepages_forced_test.go
        ├── codepages.go
        ├── codepages_iconv_unix.go
        ├── codepages_issue875_test.go
        ├── codepages_test.go
        ├── codepages_unix.go
        ├── codepages_unix_test.go
        ├── codepages_utf8_system_test.go
        ├── codepages_windows.go
        ├── codepages_windows_test.go
        ├── contributions.go
        ├── destination_overwrite_test.go
        ├── device_size_test.go
        ├── disks_unix.go
        ├── disks_unix_test.go
        ├── disks_vfs.go
        ├── disks_vfs_test.go
        ├── disks_windows.go
        ├── disks_windows_test.go
        ├── hidden_unix.go
        ├── hidden_windows.go
        ├── hostfs
        │   ├── errno_windows.go
        │   ├── hostfs_posix.go
        │   ├── hostfs_posix_test.go
        │   ├── hostfs_windows_coverage_test.go
        │   ├── hostfs_windows.go
        │   ├── hostfs_windows_test.go
        │   └── hostfs_winescape.go
        ├── hostmode
        │   ├── hostmode.go
        │   └── hostmode_test.go
        ├── hostpath
        │   ├── hostpath_posix.go
        │   └── hostpath_windows.go
        ├── isabs_test.go
        ├── lock_manager_test.go
        ├── null_vfs.go
        ├── null_vfs_test.go
        ├── os_vfs_contract_coverage_test.go
        ├── os_vfs_dot_test.go
        ├── os_vfs.go
        ├── os_vfs_junction_stub.go
        ├── os_vfs_junction_test.go
        ├── os_vfs_noreplace_test.go
        ├── os_vfs_physical_other.go
        ├── os_vfs_physical_test.go
        ├── os_vfs_physical_unix.go
        ├── os_vfs_physical_windows.go
        ├── os_vfs_platform_unix.go
        ├── os_vfs_platform_windows.go
        ├── os_vfs_posix_atimespec.go
        ├── os_vfs_posix_atim.go
        ├── os_vfs_reparse_other.go
        ├── os_vfs_reparse_windows.go
        ├── os_vfs_reparse_windows_test.go
        ├── os_vfs_search.go
        ├── os_vfs_search_test.go
        ├── os_vfs_symlink_test.go
        ├── os_vfs_test.go
        ├── os_vfs_unix_test.go
        ├── os_vfs_windows.go
        ├── os_vfs_windows_test.go
        ├── patch_inplace_test.go
        ├── privileges_windows.go
        ├── prompt_hold.go
        ├── prompt_hold_test.go
        ├── quick_view.go
        ├── quick_view_test.go
        ├── registry_vfs_windows.go
        ├── registry_vfs_windows_test.go
        ├── rename_noreplace_darwin.go
        ├── rename_noreplace.go
        ├── rename_noreplace_linux.go
        ├── rename_noreplace_test.go
        ├── rename_noreplace_unix.go
        ├── rename_noreplace_windows.go
        ├── reparse.go
        ├── reparse_test.go
        ├── scanner.go
        ├── scanner_test.go
        ├── session_identity_test.go
        ├── settings.go
        ├── share.go
        ├── share_test.go
        ├── sudo_askpass_test.go
        ├── sudo_askpass_unix.go
        ├── sudo_askpass_windows.go
        ├── sudo_client.go
        ├── sudo_client_platform_unix.go
        ├── sudo_client_platform_windows.go
        ├── sudo_client_windows_test.go
        ├── sudo_dispatcher_unix.go
        ├── sudo_dispatcher_unix_test.go
        ├── sudo_dispatcher_windows.go
        ├── sudo_ipc_unix.go
        ├── sudo_ipc_windows.go
        ├── sudo_msg.go
        ├── sudo_test.go
        ├── trash_darwin.go
        ├── trash_darwin_test.go
        ├── trash_freedesktop.go
        ├── trash_freedesktop_test.go
        ├── trash.go
        ├── trash_test.go
        ├── trash_windows.go
        ├── uri_provider.go
        ├── uri_provider_test.go
        ├── utils.go
        ├── utils_test.go
        └── vfs.go
    
    210 directories, 2118 files
