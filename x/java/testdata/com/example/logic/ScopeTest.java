package com.example.logic;

// 目标：验证 ScopeBlock 内的变量计数（$1, $2）以及 Lambda。
public class ScopeTest {
    public void test() {
        int x = 1;
        {
            int x = 2; // 验证 QN: ...test().block$1.x$1
        }
        if (true) {
            int x = 3; // 验证 QN: ...test().block$2.x$1
        }

        Runnable r = () -> {
            int x = 4; // Lambda 作用域
        };
    }
}