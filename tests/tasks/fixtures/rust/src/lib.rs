//! A dependency-free fixture.
/// Return the input unchanged.
///
/// ```
/// assert_eq!(rust_fixture::identity(7), 7);
/// ```
pub const fn identity(value: u8) -> u8 {
    value
}

#[cfg(test)]
mod tests {
    #[test]
    fn identity_preserves_value() {
        assert_eq!(super::identity(7), 7);
    }
}
