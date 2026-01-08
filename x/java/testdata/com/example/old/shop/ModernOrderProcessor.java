package com.example.shop;

import java.util.List;
import java.util.Optional;

/**
 * 挑战点：
 * 1. Record 的定义与 Accessor 隐式调用
 * 2. 紧凑构造函数 (Compact Constructor)
 * 3. 静态方法引用 (Order::price)
 * 4. 实例方法引用 (System.out::println)
 * 5. 链式调用 (Optional.of(...).map(...))
 */
public record Order(String id, double price) {
    // 1. 紧凑构造函数
    public Order {
        if (price < 0) throw new IllegalArgumentException("Negative price");
    }

    public static void process() {
        List<Order> orders = List.of(new Order("A1", 10.5));

        orders.stream()
            // 2. 方法引用 (隐式 Accessor 调用)
            .map(Order::price)
            .forEach(System.out::println); // 3. 实例方法引用 + JDK 内置对象
    }

    public void log() {
        // 4. 连缀调用 + 隐式 Accessor
        Optional.of(this.id())
            .ifPresent(System.out::println);
    }
}