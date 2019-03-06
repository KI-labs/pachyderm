#!/usr/bin/env python3

import os
import re
import sys
import glob
import time
import argparse
import datetime
import threading
import subprocess
import http.client
import collections

TEST_ROOT = os.path.join("etc", "testing", "s3gateway")
RUNS_ROOT = os.path.join(TEST_ROOT, "runs")
TRACEBACK_HEADER = "Traceback (most recent call last):\n"
RAN_PATTERN = re.compile(r"Ran (\d+) tests in [\d\.]+s")
FAILED_PATTERN = re.compile(r"FAILED \(SKIP=(\d+), errors=(\d+), failures=(\d+)\)")

class Gateway:
    def target(self):
        self.proc = subprocess.Popen(["pachctl", "s3gateway", "-v"])

    def __enter__(self):
        t = threading.Thread(target=self.target)
        t.daemon = True
        t.start()

    def __exit__(self, type, value, traceback):
        if hasattr(self, "proc"):
            self.proc.kill()

def compute_stats(filename):
    ran = 0
    skipped = 0
    errored = 0
    failed = 0

    with open(filename, "r") as f:
        for line in f:
            match = RAN_PATTERN.match(line)
            if match:
                ran = int(match.groups()[0])
                continue

            match = FAILED_PATTERN.match(line)
            if match:
                skipped = int(match.groups()[0])
                errored = int(match.groups()[1])
                failed = int(match.groups()[2])

    if ran != 0 and skipped != 0 and errored != 0 and failed != 0:
        return (ran - skipped - errored - failed, ran - skipped)
    else:
        return (0, 0)

def test_pass(no_persist, nose_args):
    proc = subprocess.run("yes | pachctl delete-all", shell=True)
    if proc.returncode != 0:
        raise Exception("bad exit code: {}".format(proc.returncode))

    with Gateway():
        for _ in range(10):
            conn = http.client.HTTPConnection("localhost:30600")

            try:
                conn.request("GET", "/")
                response = conn.getresponse()
                if response.status == 200:
                    break
            except ConnectionRefusedError:
                pass

            conn.close()
            print("Waiting for s3gateway...")
            time.sleep(1)
        else:
            print("s3gateway did not start", file=sys.stderr)
            sys.exit(1)

        args = [os.path.join("virtualenv", "bin", "nosetests"), nose_args]

        env = dict(os.environ)
        env["S3TEST_CONF"] = os.path.join("..", "s3gateway.conf")
        cwd = os.path.join(TEST_ROOT, "s3-tests")

        if no_persist:
            proc = subprocess.run(args, env=env, cwd=cwd)
        else:
            filepath = os.path.join(RUNS_ROOT, datetime.datetime.now().strftime("%Y-%m-%d-%H-%M-%S.txt"))
            with open(filepath, "w") as f:
                proc = subprocess.run(args, stderr=f, env=env, cwd=cwd)

        print("Test run exited with {}".format(proc.returncode))

def main():
    parser = argparse.ArgumentParser(description="Runs conformance tests for the s3gateway.")
    parser.add_argument("--no-run", default=False, action="store_true", help="Disables test run.")
    parser.add_argument("--no-persist", default=False, action="store_true", help="Disables persistence of test output.")
    parser.add_argument("--nose-args", default="", help="Arguments to be passed into `nosetest`")
    args = parser.parse_args()

    output = subprocess.run("ps -ef | grep pachctl | grep -v grep", shell=True, stdout=subprocess.PIPE).stdout

    if len(output.strip()) > 0:
        print("It looks like `pachctl` is already running. Please kill it before running conformance tests.", file=sys.stderr)
        sys.exit(1)

    if not args.no_run:
        test_pass(args.no_persist, args.nose_args)

    if not args.no_persist:
        log_files = sorted(glob.glob(os.path.join(RUNS_ROOT, "*.txt")))

        if len(log_files) == 0:
            print("No log files found", file=sys.stderr)
            sys.exit(1)

        old_stats = None
        if len(log_files) > 1:
            old_stats = compute_stats(log_files[-2])

        filepath = log_files[-1]
        stats = compute_stats(filepath)

        if old_stats:
            print("Overall results: {}/{} (vs last run: {}/{})".format(*stats, *old_stats))
        else:
            print("Overall results: {}/{}".format(*stats))

        in_traceback = False
        causes = collections.Counter()
        with open(filepath, "r") as f:
            for line in f:
                if line == TRACEBACK_HEADER:
                    in_traceback = True
                elif in_traceback:
                    if not line.startswith("  "):
                        causes[line.rstrip()] += 1
                        in_traceback = False

        causes = sorted(causes.items(), key=lambda i: i[1], reverse=True)
        for (cause_name, cause_count) in causes:
            print("{}: {}".format(cause_name, cause_count))
    
if __name__ == "__main__":
    main()