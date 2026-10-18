"""apply() exits the process with status 5 on removing, or is killed by a signal on opening."""
import os
import signal
import sys


def new():
    return None


def apply(door, event, env):
    if event == "remove":
        sys.exit(5)
    os.kill(os.getpid(), signal.SIGKILL)


def observe(door):
    return "closed"
