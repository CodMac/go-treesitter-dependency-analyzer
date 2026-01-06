package com.example.lombok;

import lombok.Builder;

@Builder
public class Config {
    private String host;
    private int port;
}

public class Main {
    public void run() {
        // 重点：验证内部类 Builder 及其方法 host()
        Config c = Config.builder().host("localhost").port(8080).build();
    }
}