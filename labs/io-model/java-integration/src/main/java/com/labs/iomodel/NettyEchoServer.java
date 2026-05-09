package com.labs.iomodel;

import io.netty.bootstrap.ServerBootstrap;
import io.netty.buffer.Unpooled;
import io.netty.channel.*;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.SocketChannel;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import io.netty.util.CharsetUtil;

/**
 * NettyEchoServer — explicit boss/worker topology.
 *
 * Boss group (1 thread)   == our v0_blocking accept loop
 *   Calls accept() on the server socket and hands each new channel
 *   to the worker group.  Exactly one thread; no parallelism here.
 *
 * Worker group (4 threads) == our v2_epoll event loop, replicated 4x
 *   Each thread runs its own epoll event loop over a subset of channels.
 *   No locking on the hot path — each channel is pinned to one thread.
 *
 * The ServerBootstrap wires these two groups together with a ChannelInitializer
 * that installs the EchoHandler on every accepted channel.
 */
public class NettyEchoServer {

    private static final int PORT = 9090;

    public static void main(String[] args) throws InterruptedException {
        // Boss: 1 thread — mirrors v0's single-threaded accept() loop.
        EventLoopGroup bossGroup   = new NioEventLoopGroup(1);
        // Workers: 4 threads — each runs an epoll loop (mirrors v2 × 4).
        EventLoopGroup workerGroup = new NioEventLoopGroup(4);

        try {
            ServerBootstrap b = new ServerBootstrap();
            b.group(bossGroup, workerGroup)
             .channel(NioServerSocketChannel.class)
             .childHandler(new ChannelInitializer<SocketChannel>() {
                 @Override
                 protected void initChannel(SocketChannel ch) {
                     ch.pipeline().addLast(new EchoHandler());
                 }
             })
             .option(ChannelOption.SO_BACKLOG, 512)
             .childOption(ChannelOption.SO_KEEPALIVE, true);

            ChannelFuture f = b.bind(PORT).sync();
            System.out.printf("[netty] echo server on port %d%n", PORT);
            System.out.println("[netty] boss=1 thread (accept loop)  worker=4 threads (epoll loops)");

            f.channel().closeFuture().sync();
        } finally {
            workerGroup.shutdownGracefully();
            bossGroup.shutdownGracefully();
        }
    }

    /** Reads any inbound bytes and writes back "hello\n". */
    static class EchoHandler extends ChannelInboundHandlerAdapter {
        private static final byte[] RESPONSE =
            "HTTP/1.1 200 OK\r\nContent-Length: 6\r\nConnection: close\r\n\r\nhello\n"
                .getBytes(CharsetUtil.UTF_8);

        @Override
        public void channelRead(ChannelHandlerContext ctx, Object msg) {
            // Release the inbound buffer immediately — we don't need the content.
            ((io.netty.buffer.ByteBuf) msg).release();
            ctx.writeAndFlush(Unpooled.copiedBuffer(RESPONSE))
               .addListener(ChannelFutureListener.CLOSE);
        }

        @Override
        public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
            cause.printStackTrace();
            ctx.close();
        }
    }
}
