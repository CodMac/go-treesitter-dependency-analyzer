package com.example.lombok;

import lombok.AllArgsConstructor;

@AllArgsConstructor
public class Item {
    private String id;
    private double price;
}

public class Order {
    public void create() {
        // 重点：验证带参构造函数是否能被 Resolve
        Item item = new Item("123", 99.9);
    }
}