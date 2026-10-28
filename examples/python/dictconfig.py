# Works the same in Django: put this dict in settings.py as LOGGING
# (and add "disable_existing_loggers": False).
import logging
import logging.config
import socket

LOGGING = {
    "version": 1,
    "disable_existing_loggers": False,
    "formatters": {
        "syslog": {
            "format": "%(asctime)s " + socket.gethostname() + " %(name)s[%(process)d]: %(message)s",
            "datefmt": "%b %d %H:%M:%S",
        },
    },
    "handlers": {
        "logrecv": {
            "class": "logging.handlers.SysLogHandler",
            "address": ("127.0.0.1", 5514),  # UDP
            "formatter": "syslog",
        },
    },
    "loggers": {
        "myapp": {"handlers": ["logrecv"], "level": "DEBUG"},
    },
}

logging.config.dictConfig(LOGGING)
logging.getLogger("myapp").warning("configured with dictConfig")
