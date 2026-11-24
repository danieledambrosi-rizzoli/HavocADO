0 - Update the parallel test routine in /home/kali/Havoc/test/run_parallel_tests.sh
    The batch should be running in parallel, not a batch at a time. Because the first batch takes a lot of time to complete and we waste time waiting;
    This PC has 32GBs of ram and 12CPUs; You must use the resources better.
    Optimize the tests as much as you can without breaking my machine and introducing lags.

1 - Implement the fix suggested in:
    Main Markdown Report: /home/kali/Havoc/test/report/test-report-20261124_011947.md
    Failure Analysis Report: /home/kali/Havoc/test/report/test-failure-analysis-20261124.md

2 - Fix the formatting of the MCP Server
    Bad coding: /home/kali/Havoc/teamserver/cmd/server/dispatch.go
    Must check all the other edits made by the AI for bad coding patterns.
