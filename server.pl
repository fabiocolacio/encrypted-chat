#!/bin/perl -w

use strict;

use threads;

use IO::Socket;
use IO::Socket::SSL;

my $port = shift || 8080;
my $host = "0.0.0.0";
my $proto = "tcp";
my $backlog = 5;

my $max_thread_count = shift || 200;

my $active :shared = 1;

my $server;

$SIG{INT} = \&clean_exit;

$| = 1;

sub clean_exit {
    my $tid = threads->tid;
    if ($tid == 0) {
        my $timestamp = localtime;

        print "\n[$timestamp] Shutting down server...\n";
        $active = 0;
        $server->close;

        my @threads = threads->list(threads::all);
        if (scalar @threads > 0) {
            $timestamp = localtime;
            print "[$timestamp] Fulfilling unresolved requests...\n";
            foreach my $thread (@threads) {
                $thread->join;
            }
        }

        exit 0;
    }
}

sub handle_client {
    my $client = shift;
    
    my $timestamp = localtime;
    my $peerhost = $client->peerhost;
    
    print "[$timestamp] $peerhost => connection established\n";

    my $message = "";
    my $message_size = 0;
    my $chunk_size = 256;
    my $eof = 0;

    while ($message !~ m/\r\n/) {
        $message_size += read($client, $message, $chunk_size, $message_size);
    }

    print($client
        "HTTP/1.1 200 OK\r\n\r\n
        <!doctype html>
        <html>
            <head>
                <title>hi</title>
            </head>
            <body>
                <h1>Hello World!</h1>
            </body>
        </html>");

    close($client);

    $timestamp = localtime;
    print "[$timestamp] $peerhost => connection closed\n";
}

$server = new IO::Socket::INET(
    LocalHost => $host,
    LocalPort => $port,
    Proto     => $proto,
    Listen    => $backlog
) or die "Failed to bind socket: $!";

for (;;) {
    my $thread_count = threads->list(threads::running);
    if ($active && $thread_count < $max_thread_count) {
        my $client = $server->accept;
        threads->new(\&handle_client, $client);
    }
}

