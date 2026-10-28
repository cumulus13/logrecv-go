<?php
// config/logging.php  (Laravel 10+ / Monolog 3)

use Monolog\Formatter\LineFormatter;
use Monolog\Handler\SyslogUdpHandler;
use Monolog\Processor\PsrLogMessageProcessor;

return [
    // ...
    'channels' => [
        // ...
        'logrecv' => [
            'driver'  => 'monolog',
            'level'   => env('LOG_LEVEL', 'debug'),
            'handler' => SyslogUdpHandler::class,
            'handler_with' => [
                'host'  => env('LOGRECV_HOST', '127.0.0.1'),
                'port'  => (int) env('LOGRECV_PORT', 5514),
                'ident' => env('LOGRECV_IDENT', 'laravel'), // shown as the app name (no spaces)
            ],
            // Laravel's default formatter adds a "[date]" prefix; logrecv already shows the time.
            'formatter' => LineFormatter::class,
            'formatter_with' => [
                'format' => '%channel%.%level_name%: %message% %context% %extra%',
                'allowInlineLineBreaks' => true,
                'ignoreEmptyContextAndExtra' => true,
                'includeStacktraces' => true,
            ],
            'processors' => [PsrLogMessageProcessor::class],
        ],
    ],
];
