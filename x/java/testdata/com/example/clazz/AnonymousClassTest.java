package com.example.clazz;

// 目标：验证 AnonymousClass 和嵌套结构。
public class AnonymousClassTest {
    public void run() {
        // 匿名内部类
        Comparable<String> cmp = new Comparable<String>() {
            @Override
            public int compareTo(String o) {
                return 0;
            }
        };
    }

    // 内部类
    static class Inner {
        enum Color { RED, BLUE }
    }
}